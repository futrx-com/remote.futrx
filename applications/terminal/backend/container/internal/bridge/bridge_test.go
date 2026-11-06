//go:build linux

package bridge

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

// startService answers on a Unix socket like remote-terminal serve. Output
// waits until released, standing in for a shell that has printed nothing.
func startService(t *testing.T) (socket string, release chan struct{}) {
	t.Helper()
	socket = filepath.Join(t.TempDir(), "terminal.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	release = make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /output", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, `{"offset":`+r.URL.Query().Get("offset")+`}`)
	})
	mux.HandleFunc("POST /input", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write(body)
	})
	server := &http.Server{Handler: mux}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })
	return socket, release
}

type harness struct {
	t         *testing.T
	in        *io.PipeWriter
	responses *bufio.Scanner
	done      chan error
}

func runBridge(t *testing.T, socket string) *harness {
	t.Helper()
	inReader, in := io.Pipe()
	outReader, out := io.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- Run(inReader, out, socket)
		out.Close()
	}()
	return &harness{t: t, in: in, responses: bufio.NewScanner(outReader), done: done}
}

func (h *harness) send(request Request) {
	h.t.Helper()
	line, _ := json.Marshal(request)
	if _, err := h.in.Write(append(line, '\n')); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) next() Response {
	h.t.Helper()
	if !h.responses.Scan() {
		h.t.Fatalf("bridge stopped answering: %v", h.responses.Err())
	}
	var response Response
	if err := json.Unmarshal(h.responses.Bytes(), &response); err != nil {
		h.t.Fatal(err)
	}
	return response
}

func TestAWaitingReadDoesNotHoldUpInput(t *testing.T) {
	socket, release := startService(t)
	bridge := runBridge(t, socket)
	bridge.send(Request{ID: 1, Method: "GET", Path: "/output", Query: "session=chat-1&generation=1&offset=7"})
	bridge.send(Request{ID: 2, Method: "POST", Path: "/input", Query: "session=chat-1&generation=1", Body: []byte("ls\r")})
	if typed := bridge.next(); typed.ID != 2 || typed.Status != http.StatusAccepted || string(typed.Body) != "ls\r" {
		t.Fatalf("input answer = %+v", typed)
	}
	close(release)
	if read := bridge.next(); read.ID != 1 || read.Status != 200 || string(read.Body) != `{"offset":7}` {
		t.Fatalf("output answer = %+v", read)
	}
	bridge.in.Close()
	select {
	case err := <-bridge.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the bridge did not stop when its input ended")
	}
}

func TestOnlyTerminalRoutesAreForwarded(t *testing.T) {
	socket, _ := startService(t)
	bridge := runBridge(t, socket)
	for id, request := range map[uint64]Request{
		1: {Method: "GET", Path: "/health"},
		2: {Method: "GET", Path: "/../etc/passwd"},
		3: {Method: "DELETE", Path: "/input"},
	} {
		request.ID = id
		bridge.send(request)
		if response := bridge.next(); response.ID != id || response.Status != http.StatusNotFound {
			t.Fatalf("request %d answered %+v", id, response)
		}
	}
}

func TestAStoppedServiceIsReportedPerRequest(t *testing.T) {
	bridge := runBridge(t, filepath.Join(t.TempDir(), "missing.sock"))
	bridge.send(Request{ID: 9, Method: "POST", Path: "/sessions", Body: []byte(`{}`)})
	if response := bridge.next(); response.ID != 9 || response.Status != http.StatusBadGateway {
		t.Fatalf("answer = %+v", response)
	}
}

func TestAbandonedReadsEndWithTheBridge(t *testing.T) {
	socket, _ := startService(t)
	bridge := runBridge(t, socket)
	bridge.send(Request{ID: 1, Method: "GET", Path: "/output", Query: "offset=0"})
	time.Sleep(50 * time.Millisecond)
	bridge.in.Close()
	go func() {
		for bridge.responses.Scan() {
		}
	}()
	select {
	case <-bridge.done:
	case <-time.After(5 * time.Second):
		t.Fatal("a waiting read kept the bridge running")
	}
}
