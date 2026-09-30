// Package parse converts Go source into a JSON-friendly description of its
// structs and the references between them, and writes edited structs back.
package parse

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Type describes a field's type: the source literal plus the named
// (non-primitive) types it refers to.
type Type struct {
	Literal string   `json:"literal"`
	Structs []string `json:"structs"`
}

type ClientStruct struct {
	Packages []Package `json:"packages"`
	Edges    []Edge    `json:"edges"`
}

type Package struct {
	Name  string `json:"name"`
	Files []File `json:"files"`
}

type File struct {
	Name    string   `json:"name"`
	Structs []Struct `json:"structs"`
}

type Struct struct {
	Name string `json:"name"`
	// TypeParams holds generic parameters as source, e.g. "[T any]".
	TypeParams string  `json:"typeParams,omitempty"`
	Fields     []Field `json:"fields"`
}

type Field struct {
	Name     string `json:"name"`
	Type     Type   `json:"type"`
	Embedded bool   `json:"embedded,omitempty"`
	// Tag is the raw struct tag including backquotes, e.g. `json:"id"`.
	Tag string `json:"tag,omitempty"`
}

type Node struct {
	FieldTypeName string `json:"fieldTypeName"`
	StructName    string `json:"structName"`
	PackageName   string `json:"packageName"`
	FileName      string `json:"fileName"`
}

type Edge struct {
	To   *Node `json:"to"`
	From *Node `json:"from"`
}

// skipDirs are directory names never scanned for Go source.
var skipDirs = map[string]bool{
	"app": true, "node_modules": true, "vendor": true, "testdata": true,
}

func exprString(fset *token.FileSet, e ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, e); err != nil {
		return fmt.Sprintf("%T", e)
	}
	return buf.String()
}

// GetStructsFile extracts the structs declared in one file and the edges
// from each field to the named types it references.
func GetStructsFile(fset *token.FileSet, f *ast.File, fname string, packageName string) (File, []Edge) {
	structs := []Struct{}
	edges := []Edge{}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.TYPE {
			continue
		}
		for _, s := range g.Specs {
			ts, ok := s.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			fields := []Field{}
			for _, field := range st.Fields.List {
				literal := exprString(fset, field.Type)
				names, toNodes := GetTypes(field.Type, packageName)
				fieldType := Type{Literal: literal, Structs: names}

				// Embedded fields have no names; use the type name, as Go does.
				fieldNames := []string{}
				for _, n := range field.Names {
					fieldNames = append(fieldNames, n.Name)
				}
				tag := ""
				if field.Tag != nil {
					tag = field.Tag.Value
				}
				embedded := len(fieldNames) == 0
				if embedded {
					fieldNames = append(fieldNames, embeddedName(field.Type))
				}

				for _, name := range fieldNames {
					fields = append(fields, Field{Name: name, Type: fieldType, Embedded: embedded, Tag: tag})
					for _, toNode := range toNodes {
						to := *toNode
						edges = append(edges, Edge{
							From: &Node{FieldTypeName: name, StructName: ts.Name.Name, FileName: fname, PackageName: packageName},
							To:   &to,
						})
					}
				}
			}
			structs = append(structs, Struct{Name: ts.Name.Name, TypeParams: typeParamsString(fset, ts.TypeParams), Fields: fields})
		}
	}
	return File{Name: fname, Structs: structs}, edges
}

func typeParamsString(fset *token.FileSet, fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	parts := []string{}
	for _, f := range fl.List {
		names := []string{}
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
		parts = append(parts, strings.Join(names, ", ")+" "+exprString(fset, f.Type))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// parseTypeParams turns "[K comparable, V any]" back into an AST field list.
func parseTypeParams(src string) (*ast.FieldList, error) {
	if src == "" {
		return nil, nil
	}
	f, err := parser.ParseFile(token.NewFileSet(), "", "package p\ntype _T"+src+" struct{}\n", 0)
	if err != nil {
		return nil, fmt.Errorf("invalid type parameters %q: %w", src, err)
	}
	return f.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).TypeParams, nil
}

func embeddedName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return embeddedName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return embeddedName(t.X)
	case *ast.IndexListExpr:
		return embeddedName(t.X)
	}
	return "_"
}

func fileIndex(pkgs []Package) map[string]string {
	idx := map[string]string{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, st := range file.Structs {
				idx[pkg.Name+"."+st.Name] = file.Name
			}
		}
	}
	return idx
}

func getPackagesEdgesDirName(path string, fset *token.FileSet) ([]Package, []Edge, map[string]*ast.Package, error) {
	var packages []Package
	var edges []Edge
	//nolint:staticcheck // ParseDir is deprecated but still the simplest fit here.
	packagemap, err := parser.ParseDir(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, nil, err
	}
	names := make([]string, 0, len(packagemap))
	for name := range packagemap {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, packagename := range names {
		// Packages are keyed by name, so every "main" package would collide.
		if packagename == "main" {
			continue
		}
		packageval := packagemap[packagename]
		fnames := make([]string, 0, len(packageval.Files))
		for fname := range packageval.Files {
			fnames = append(fnames, fname)
		}
		sort.Strings(fnames)
		files := []File{}
		for _, fname := range fnames {
			newfile, newedges := GetStructsFile(fset, packageval.Files[fname], fname, packagename)
			files = append(files, newfile)
			edges = append(edges, newedges...)
		}
		packages = append(packages, Package{Name: packagename, Files: files})
	}
	return packages, edges, packagemap, nil
}

// GetStructsDirName parses every Go package under path.
func GetStructsDirName(path string) (*ClientStruct, map[string]*ast.Package, error) {
	packages := []Package{}
	edges := []Edge{}
	pkgmap := map[string]*ast.Package{}
	fset := token.NewFileSet()

	directories := []string{}
	err := filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if p != path && (skipDirs[name] || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
			return filepath.SkipDir
		}
		directories = append(directories, p)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	var parseErrs []string
	for _, directory := range directories {
		newpackages, newedges, newpkgmap, err := getPackagesEdgesDirName(directory, fset)
		if err != nil {
			// Keep going: one broken file shouldn't hide the rest of the project.
			parseErrs = append(parseErrs, err.Error())
			continue
		}
		packages = append(packages, newpackages...)
		edges = append(edges, newedges...)
		for k, v := range newpkgmap {
			pkgmap[k] = v
		}
	}

	// Edge targets only know their package while walking the AST; fill in
	// the file now. Targets outside the project (library types) are dropped.
	idx := fileIndex(packages)
	validedges := []Edge{}
	for _, edge := range edges {
		if name, ok := idx[edge.To.PackageName+"."+edge.To.StructName]; ok {
			edge.To.FileName = name
			validedges = append(validedges, edge)
		}
	}

	var outErr error
	if len(parseErrs) > 0 {
		outErr = fmt.Errorf("some files could not be parsed: %s", strings.Join(parseErrs, "; "))
	}
	return &ClientStruct{Packages: packages, Edges: validedges}, pkgmap, outErr
}

func isPrimitive(name string) bool {
	for _, basicType := range types.Typ {
		if name == basicType.Name() {
			return true
		}
	}
	switch name {
	case "error", "byte", "rune", "any", "comparable":
		return true
	}
	return false
}

// GetType returns the name of an identifier type and, if it is not a
// primitive, a node pointing at it (file is resolved later).
func GetType(node *ast.Ident, packageName string) (string, *Node) {
	var toNode *Node
	name := node.Name
	if !isPrimitive(name) {
		toNode = &Node{StructName: name, PackageName: packageName}
	}
	return name, toNode
}

// GetTypes returns every named type referenced by a type expression.
// It never panics: unknown expression kinds simply yield no references.
func GetTypes(node ast.Expr, packageName string) ([]string, []*Node) {
	switch t := node.(type) {
	case *ast.Ident:
		name, toNode := GetType(t, packageName)
		if toNode != nil {
			return []string{name}, []*Node{toNode}
		}
		return []string{name}, nil
	case *ast.SelectorExpr:
		pkg, ok := t.X.(*ast.Ident)
		if !ok {
			return []string{t.Sel.Name}, nil
		}
		name, toNode := GetType(t.Sel, pkg.Name)
		if toNode != nil {
			return []string{name}, []*Node{toNode}
		}
		return []string{name}, nil
	case *ast.ArrayType:
		return GetTypes(t.Elt, packageName)
	case *ast.Ellipsis:
		return GetTypes(t.Elt, packageName)
	case *ast.StarExpr:
		return GetTypes(t.X, packageName)
	case *ast.ParenExpr:
		return GetTypes(t.X, packageName)
	case *ast.ChanType:
		return GetTypes(t.Value, packageName)
	case *ast.MapType:
		return mergeTypes(packageName, t.Key, t.Value)
	case *ast.IndexExpr: // generic instantiation: T[A]
		return mergeTypes(packageName, t.X, t.Index)
	case *ast.IndexListExpr: // generic instantiation: T[A, B]
		return mergeTypes(packageName, append([]ast.Expr{t.X}, t.Indices...)...)
	case *ast.StructType:
		var exprs []ast.Expr
		for _, f := range t.Fields.List {
			exprs = append(exprs, f.Type)
		}
		return mergeTypes(packageName, exprs...)
	case *ast.FuncType, *ast.InterfaceType:
		return nil, nil
	default:
		return nil, nil
	}
}

func mergeTypes(packageName string, exprs ...ast.Expr) ([]string, []*Node) {
	var names []string
	var nodes []*Node
	for _, e := range exprs {
		n, nd := GetTypes(e, packageName)
		names = append(names, n...)
		nodes = append(nodes, nd...)
	}
	return names, nodes
}

// WriteClientPackages applies struct edits from the client to the parsed
// packages and rewrites the affected files on disk.
func WriteClientPackages(dirpath string, pkgs map[string]*ast.Package, clientpackages []Package) error {
	for _, clientpackage := range clientpackages {
		packageast, ok := pkgs[clientpackage.Name]
		if !ok {
			return fmt.Errorf("unknown package %q", clientpackage.Name)
		}
		for _, clientfile := range clientpackage.Files {
			f, ok := packageast.Files[clientfile.Name]
			if !ok {
				return fmt.Errorf("unknown file %q in package %q", clientfile.Name, clientpackage.Name)
			}
			newFile, err := clientFileToAST(clientfile, f)
			if err != nil {
				return err
			}
			if err := writeFileAST(clientfile.Name, newFile); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeFileAST(path string, f *ast.File) error {
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), f); err != nil {
		return err
	}
	out, err := format.Source(spaceTopLevelDecls(buf.Bytes()))
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// spaceTopLevelDecls restores the blank line gofmt-style code has between
// top-level declarations; printing a rebuilt AST without positions drops it.
// Declaration starts come from re-parsing, so string literals are never touched.
func spaceTopLevelDecls(src []byte) []byte {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return src
	}
	starts := map[int]bool{}
	for _, d := range f.Decls {
		starts[fset.Position(d.Pos()).Line] = true
	}
	lines := strings.Split(string(src), "\n")
	out := make([]string, 0, len(lines)+len(starts))
	for i, line := range lines {
		if starts[i+1] && i > 0 && strings.TrimSpace(lines[i-1]) != "" {
			out = append(out, "")
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}

// clientFileToAST replaces the struct declarations in f with the client's
// structs. Non-struct type declarations, imports, funcs, vars and consts are
// kept in place; the new structs go where the first struct used to be (or
// after the imports if the file had none).
func clientFileToAST(clientfile File, f *ast.File) (*ast.File, error) {
	newstructs, err := clientFileToDecls(clientfile)
	if err != nil {
		return nil, err
	}

	decls := []ast.Decl{}
	inserted := false
	for _, decl := range f.Decls {
		g, ok := decl.(*ast.GenDecl)
		if !ok || g.Tok != token.TYPE {
			decls = append(decls, decl)
			continue
		}
		kept := []ast.Spec{}
		hadStruct := false
		for _, s := range g.Specs {
			if ts, ok := s.(*ast.TypeSpec); ok {
				if _, isStruct := ts.Type.(*ast.StructType); isStruct {
					hadStruct = true
					continue
				}
			}
			kept = append(kept, s)
		}
		if hadStruct && !inserted {
			decls = append(decls, newstructs...)
			inserted = true
		}
		if len(kept) > 0 {
			g.Specs = kept
			decls = append(decls, g)
		}
	}

	if !inserted {
		// No existing structs: place new ones after the import block(s).
		pos := 0
		for pos < len(decls) {
			if g, ok := decls[pos].(*ast.GenDecl); ok && g.Tok == token.IMPORT {
				pos++
				continue
			}
			break
		}
		rest := append([]ast.Decl{}, decls[pos:]...)
		decls = append(append(decls[:pos], newstructs...), rest...)
	}

	f.Decls = decls
	// Comments are position-based and would be misplaced after the rewrite.
	f.Comments = nil
	return f, nil
}

// clientFileToDecls builds one type declaration per client struct.
func clientFileToDecls(clientfile File) ([]ast.Decl, error) {
	decls := []ast.Decl{}
	for _, clientstruct := range clientfile.Structs {
		if !token.IsIdentifier(clientstruct.Name) {
			return nil, fmt.Errorf("invalid struct name %q", clientstruct.Name)
		}
		fieldList := []*ast.Field{}
		for _, clientfield := range clientstruct.Fields {
			if !clientfield.Embedded && !token.IsIdentifier(clientfield.Name) {
				return nil, fmt.Errorf("invalid field name %q in struct %s", clientfield.Name, clientstruct.Name)
			}
			parsedtype, err := parser.ParseExpr(clientfield.Type.Literal)
			if err != nil {
				return nil, fmt.Errorf("invalid type %q for %s.%s: %w", clientfield.Type.Literal, clientstruct.Name, clientfield.Name, err)
			}
			field := &ast.Field{Type: parsedtype}
			if clientfield.Tag != "" {
				if !strings.HasPrefix(clientfield.Tag, "`") && !strings.HasPrefix(clientfield.Tag, "\"") {
					return nil, fmt.Errorf("invalid tag %q for %s.%s", clientfield.Tag, clientstruct.Name, clientfield.Name)
				}
				field.Tag = &ast.BasicLit{Kind: token.STRING, Value: clientfield.Tag}
			}
			if !clientfield.Embedded {
				field.Names = []*ast.Ident{ast.NewIdent(clientfield.Name)}
			}
			fieldList = append(fieldList, field)
		}
		typeParams, err := parseTypeParams(clientstruct.TypeParams)
		if err != nil {
			return nil, err
		}
		decls = append(decls, &ast.GenDecl{
			Tok: token.TYPE,
			Specs: []ast.Spec{&ast.TypeSpec{
				Name:       ast.NewIdent(clientstruct.Name),
				TypeParams: typeParams,
				Type:       &ast.StructType{Fields: &ast.FieldList{List: fieldList}},
			}},
		})
	}
	return decls, nil
}
