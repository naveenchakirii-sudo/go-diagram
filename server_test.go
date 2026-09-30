package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/grant/go-diagram/parse"
)

func TestWebsocketPushesDiagramAndSavesEdits(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "demo.go")
	src := "package demo\n\ntype Node struct {\n\tValue int\n}\n"
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(wsHandler(&project{dir: dir}))
	defer srv.Close()
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	ws.SetReadDeadline(time.Now().Add(10 * time.Second))

	var cs parse.ClientStruct
	if err := ws.ReadJSON(&cs); err != nil {
		t.Fatal(err)
	}
	if len(cs.Packages) != 1 || cs.Packages[0].Files[0].Structs[0].Name != "Node" {
		t.Fatalf("unexpected initial diagram: %+v", cs)
	}

	// Rename the struct from the "browser" and expect the file to change.
	cs.Packages[0].Files[0].Structs[0].Name = "Vertex"
	if err := ws.WriteJSON(cs); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(file)
		if strings.Contains(string(b), "type Vertex struct") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, _ := os.ReadFile(file)
	t.Fatalf("edit was not written to disk:\n%s", b)
}
