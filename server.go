// Command go-diagram serves an editable UML-style diagram of the structs in a
// Go project and writes edits made in the browser back to the source files.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"go/ast"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/grant/go-diagram/parse"
)

type clientError struct {
	Error string `json:"error"`
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	filePeriod = 2 * time.Second
)

var (
	addr      = flag.String("addr", "127.0.0.1:8080", "address to listen on (use :8080 to expose on all interfaces)")
	staticDir = flag.String("static", "./app/dist", "directory containing the built frontend")
	noBrowser = flag.Bool("no-browser", false, "don't open a browser on start")
	readOnly  = flag.Bool("read-only", false, "view diagrams without writing edits back to disk")

	upgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}
)

// project holds the parsed state of the directory being diagrammed. It is
// shared by all connections, so access goes through the mutex.
type project struct {
	dir string

	mu   sync.Mutex
	pkgs map[string]*ast.Package
}

// fingerprint summarises every .go file's path, size and mtime, so added,
// removed and modified files are all detected.
func (p *project) fingerprint() string {
	var entries []string
	filepath.WalkDir(p.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if path != p.dir && (name == "node_modules" || name == "vendor" || name == "app" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") {
			if info, err := d.Info(); err == nil {
				entries = append(entries, fmt.Sprintf("%s|%d|%d", path, info.Size(), info.ModTime().UnixNano()))
			}
		}
		return nil
	})
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:])
}

func (p *project) parse() (*parse.ClientStruct, error) {
	cs, pkgs, err := parse.GetStructsDirName(p.dir)
	if cs != nil {
		p.mu.Lock()
		p.pkgs = pkgs
		p.mu.Unlock()
	}
	return cs, err
}

func (p *project) write(pkgs []parse.Package) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return parse.WriteClientPackages(p.dir, p.pkgs, pkgs)
}

// session handles one browser connection. All writes to the socket happen in
// run(), since gorilla/websocket allows only one concurrent writer.
type session struct {
	proj   *project
	ws     *websocket.Conn
	errors chan error
}

func (s *session) readLoop() {
	defer close(s.errors)
	s.ws.SetReadLimit(4 << 20)
	s.ws.SetReadDeadline(time.Now().Add(pongWait))
	s.ws.SetPongHandler(func(string) error { return s.ws.SetReadDeadline(time.Now().Add(pongWait)) })
	for {
		var msg parse.ClientStruct
		if err := s.ws.ReadJSON(&msg); err != nil {
			if !websocket.IsCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Println("read:", err)
			}
			return
		}
		if *readOnly {
			s.errors <- fmt.Errorf("server is in read-only mode; edits were not saved")
			continue
		}
		if err := s.proj.write(msg.Packages); err != nil {
			s.errors <- err
			continue
		}
		log.Println("saved client edits")
	}
}

func (s *session) send(v any) error {
	s.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return s.ws.WriteJSON(v)
}

func (s *session) run() {
	pingTicker := time.NewTicker(pingPeriod)
	fileTicker := time.NewTicker(filePeriod)
	defer func() {
		pingTicker.Stop()
		fileTicker.Stop()
		s.ws.Close()
	}()

	lastPrint := ""
	push := func() bool {
		fp := s.proj.fingerprint()
		if fp == lastPrint {
			return true
		}
		lastPrint = fp
		cs, err := s.proj.parse()
		if err != nil {
			log.Println("parse:", err)
			if sendErr := s.send(clientError{Error: err.Error()}); sendErr != nil {
				return false
			}
		}
		if cs != nil {
			return s.send(cs) == nil
		}
		return true
	}

	if !push() {
		return
	}
	for {
		select {
		case err, ok := <-s.errors:
			if !ok {
				return // reader finished: client went away
			}
			if s.send(clientError{Error: err.Error()}) != nil {
				return
			}
			// The edit was rejected, so resend the real state to undo it in the UI.
			lastPrint = ""
			if !push() {
				return
			}
		case <-fileTicker.C:
			if !push() {
				return
			}
		case <-pingTicker.C:
			s.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := s.ws.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func wsHandler(proj *project) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade:", err)
			return
		}
		s := &session{proj: proj, ws: ws, errors: make(chan error, 8)}
		go s.readLoop()
		s.run()
	}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("open", url)
	}
	_ = cmd.Start()
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: go-diagram [flags] <directory>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	dir, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		log.Fatalf("%s is not a directory", dir)
	}
	if _, err := os.Stat(filepath.Join(*staticDir, "index.html")); err != nil {
		log.Fatalf("frontend not found in %s; run `npm run build` in app/ or pass -static", *staticDir)
	}

	proj := &project{dir: dir}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(*staticDir)))
	mux.HandleFunc("/ws", wsHandler(proj))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })

	url := "http://" + strings.Replace(*addr, "0.0.0.0", "localhost", 1)
	if strings.HasPrefix(*addr, ":") {
		url = "http://localhost" + *addr
	}
	log.Printf("diagramming %s", dir)
	log.Printf("listening on %s", url)
	if !*noBrowser {
		go openBrowser(url)
	}
	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
