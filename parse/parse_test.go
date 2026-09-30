package parse

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func findStruct(cs *ClientStruct, name string) *Struct {
	for _, p := range cs.Packages {
		for _, f := range p.Files {
			for i := range f.Structs {
				if f.Structs[i].Name == name {
					return &f.Structs[i]
				}
			}
		}
	}
	return nil
}

func TestParseStructsAndEdges(t *testing.T) {
	dir := writeTemp(t, map[string]string{"graph.go": `package graph

type Node struct {
	Value float32
}

type Edge struct {
	Weight     int
	Start, End *Node
}

type Graph struct {
	Nodes []*Node
	Edges []Edge
	Index map[string]*Node
}
`})
	cs, _, err := GetStructsDirName(dir)
	if err != nil {
		t.Fatal(err)
	}
	edge := findStruct(cs, "Edge")
	if edge == nil || len(edge.Fields) != 3 {
		t.Fatalf("Edge should have 3 fields (multi-name field expanded), got %+v", edge)
	}
	// Start, End, Nodes, Edges, Index(value) => 5 edges.
	if len(cs.Edges) != 5 {
		t.Fatalf("expected 5 edges, got %d", len(cs.Edges))
	}
	for _, e := range cs.Edges {
		if e.To.FileName == "" {
			t.Errorf("edge target file not resolved: %+v", e.To)
		}
	}
}

func TestModernSyntaxDoesNotPanic(t *testing.T) {
	dir := writeTemp(t, map[string]string{"gen.go": `package gen

import "sync"

type Item struct{ ID int }

type Base struct{}

type List[T any] struct {
	Items []T
}

type Pair[K comparable, V any] struct {
	Key K
	Val V
}

type Store struct {
	Base
	*sync.Mutex
	Cache    List[Item]
	Lookup   Pair[string, Item]
	Anon     struct{ Inner Item }
	Handler  func(Item) error
	Events   chan Item
	Iface    interface{ Close() error }
	Variadic func(...Item)
}
`})
	cs, _, err := GetStructsDirName(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := findStruct(cs, "Store")
	if store == nil {
		t.Fatal("Store not found")
	}
	if !store.Fields[0].Embedded || store.Fields[0].Name != "Base" {
		t.Errorf("embedded field not detected: %+v", store.Fields[0])
	}
	targets := map[string]int{}
	for _, e := range cs.Edges {
		if e.From.StructName == "Store" {
			targets[e.To.StructName]++
		}
	}
	for _, want := range []string{"Base", "List", "Item", "Pair"} {
		if targets[want] == 0 {
			t.Errorf("expected an edge from Store to %s, got %v", want, targets)
		}
	}
}

func TestSkipsNonSourceDirs(t *testing.T) {
	dir := writeTemp(t, map[string]string{
		"lib/lib.go":          "package lib\ntype A struct{}\n",
		"application/app.go":  "package application\ntype B struct{}\n",
		"node_modules/x/x.go": "package x\ntype C struct{}\n",
		".hidden/h.go":        "package hidden\ntype D struct{}\n",
		"vendor/v/v.go":       "package v\ntype E struct{}\n",
	})
	cs, _, err := GetStructsDirName(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"A", "B"} {
		if findStruct(cs, name) == nil {
			t.Errorf("struct %s should be found", name)
		}
	}
	for _, name := range []string{"C", "D", "E"} {
		if findStruct(cs, name) != nil {
			t.Errorf("struct %s should be skipped", name)
		}
	}
}

func TestBrokenFileDoesNotHideOthers(t *testing.T) {
	dir := writeTemp(t, map[string]string{
		"good/good.go": "package good\ntype Ok struct{}\n",
		"bad/bad.go":   "package bad\ntype Broken struct{\n",
	})
	cs, _, err := GetStructsDirName(dir)
	if err == nil {
		t.Error("expected a parse error to be reported")
	}
	if findStruct(cs, "Ok") == nil {
		t.Error("valid package should still be returned")
	}
}

// Round-trip: editing a struct must not duplicate structs or drop other code.
// The original implementation duplicated the first struct and deleted the
// first function when a file had no imports.
func TestWriteBackPreservesOtherDecls(t *testing.T) {
	cases := map[string]string{
		"no imports": `package demo

type Edge struct {
	Weight int
}

type ID string

func Hello() string { return "hi" }

type Node struct {
	Value float32
}
`,
		"with imports": `package demo

import "fmt"

type Edge struct {
	Weight int
}

type ID string

func Hello() string { return fmt.Sprint("hi") }

type Node struct {
	Value float32
}
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeTemp(t, map[string]string{"demo.go": src})
			cs, pkgs, err := GetStructsDirName(dir)
			if err != nil {
				t.Fatal(err)
			}
			// Simulate a client edit: rename a field.
			cs.Packages[0].Files[0].Structs[0].Fields[0].Name = "Cost"
			if err := WriteClientPackages(dir, pkgs, cs.Packages); err != nil {
				t.Fatal(err)
			}
			out, err := os.ReadFile(filepath.Join(dir, "demo.go"))
			if err != nil {
				t.Fatal(err)
			}
			got := string(out)
			if n := strings.Count(got, "type Edge struct"); n != 1 {
				t.Errorf("Edge declared %d times:\n%s", n, got)
			}
			for _, want := range []string{"Cost", "type ID string", "func Hello()", "type Node struct"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q after write-back:\n%s", want, got)
				}
			}
			// The rewritten file must still parse.
			if _, _, err := GetStructsDirName(dir); err != nil {
				t.Errorf("rewritten file no longer parses: %v\n%s", err, got)
			}
		})
	}
}

func TestWriteBackRejectsInvalidInput(t *testing.T) {
	dir := writeTemp(t, map[string]string{"demo.go": "package demo\ntype A struct{ X int }\n"})
	cs, pkgs, err := GetStructsDirName(dir)
	if err != nil {
		t.Fatal(err)
	}
	cs.Packages[0].Files[0].Structs[0].Fields[0].Type.Literal = "[[[not a type"
	if err := WriteClientPackages(dir, pkgs, cs.Packages); err == nil {
		t.Error("expected error for invalid type literal")
	}
	cs.Packages[0].Files[0].Structs[0].Fields[0].Type.Literal = "int"
	cs.Packages[0].Files[0].Structs[0].Name = "bad name"
	if err := WriteClientPackages(dir, pkgs, cs.Packages); err == nil {
		t.Error("expected error for invalid struct name")
	}
	if err := WriteClientPackages(dir, pkgs, []Package{{Name: "nope"}}); err == nil {
		t.Error("expected error for unknown package")
	}
}

func TestWriteBackKeepsGenericsAndTags(t *testing.T) {
	src := "package demo\n\nconst doc = `\ntype NotADecl struct{}\n`\n\ntype User struct {\n\tID   int    `json:\"id\"`\n\tName string `json:\"name,omitempty\"`\n}\n\ntype Pair[K comparable, V any] struct {\n\tKey K\n\tVal V\n}\n"
	dir := writeTemp(t, map[string]string{"demo.go": src})
	cs, pkgs, err := GetStructsDirName(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := cs.Packages[0].Files[0].Structs[1].TypeParams; got != "[K comparable, V any]" {
		t.Fatalf("type params = %q", got)
	}
	cs.Packages[0].Files[0].Structs[0].Fields[1].Name = "FullName"
	if err := WriteClientPackages(dir, pkgs, cs.Packages); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "demo.go"))
	got := string(out)
	if !strings.Contains(got, "`\ntype NotADecl struct{}\n`") {
		t.Errorf("raw string literal was modified:\n%s", got)
	}
	if !strings.Contains(got, "}\n\ntype Pair") {
		t.Errorf("expected a blank line between declarations:\n%s", got)
	}
	for _, want := range []string{"`json:\"id\"`", "FullName string `json:\"name,omitempty\"`", "type Pair[K comparable, V any] struct"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q after write-back:\n%s", want, got)
		}
	}
}
