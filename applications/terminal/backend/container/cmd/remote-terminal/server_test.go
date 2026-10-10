//go:build linux

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestShellOutputReachesTheClient(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	client := connect(t, server, "session=chat-1")
	client.input("echo he''llo\n")
	client.until("hello")
}

func TestBinaryFramesAreTypedIntoTheShell(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	client := connect(t, server, "session=chat-1")
	client.send(opBinary, []byte("echo bi''nary\n"))
	client.until("binary")
}

func TestReconnectingReturnsToTheSameShellWithItsOutput(t *testing.T) {
	server, manager := startServer(t, sessionConfig{})
	first := connect(t, server, "session=chat-1")
	first.input("MARKER=kept; echo fi''rst\n")
	first.until("first")
	first.conn.Close()
	waitFor(t, "the viewer to detach", func() bool {
		return viewerCount(manager, "chat-1") == 0
	})

	second := connect(t, server, "session=chat-1")
	// The replay shows what the dropped connection had already printed, and
	// the variable proves it is the same shell rather than a new one.
	second.until("first")
	second.input("echo va''lue=$MARKER\n")
	second.until("value=kept")
}

func TestSessionsAreSeparateShells(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	first := connect(t, server, "session=chat-1")
	first.input("MARKER=one; echo re''ady\n")
	first.until("ready")

	second := connect(t, server, "session=chat-2")
	second.input("echo va''lue=[$MARKER]\n")
	second.until("value=[]")
}

func TestShellExitClosesWithTheExitCodeAndForgetsTheSession(t *testing.T) {
	server, manager := startServer(t, sessionConfig{})
	client := connect(t, server, "session=chat-1")
	client.input("exit\n")
	if code := client.closeCode(); code != closeShellExited {
		t.Fatalf("close code = %d, want %d", code, closeShellExited)
	}
	waitFor(t, "the session to be forgotten", func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		return len(manager.sessions) == 0
	})
}

func TestUnwatchedSessionIsEndedAfterTheGracePeriod(t *testing.T) {
	server, manager := startServer(t, sessionConfig{detachedTTL: 50 * time.Millisecond})
	client := connect(t, server, "session=chat-1")
	client.input("echo re''ady\n")
	client.until("ready")
	client.conn.Close()
	waitFor(t, "the unwatched session to end", func() bool {
		manager.mu.Lock()
		defer manager.mu.Unlock()
		return len(manager.sessions) == 0
	})
}

func TestReattachingBeforeTheGracePeriodKeepsTheSession(t *testing.T) {
	server, manager := startServer(t, sessionConfig{detachedTTL: 300 * time.Millisecond})
	first := connect(t, server, "session=chat-1")
	first.input("MARKER=kept; echo re''ady\n")
	first.until("ready")
	first.conn.Close()
	waitFor(t, "the viewer to detach", func() bool {
		return viewerCount(manager, "chat-1") == 0
	})

	second := connect(t, server, "session=chat-1")
	time.Sleep(500 * time.Millisecond)
	second.input("echo va''lue=$MARKER\n")
	second.until("value=kept")
}

func TestShellStartsInTheRequestedWorkspaceDirectory(t *testing.T) {
	workspace := realPath(t, t.TempDir())
	if err := os.Mkdir(filepath.Join(workspace, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	server, _ := startServer(t, sessionConfig{workspace: workspace})
	client := connect(t, server, "session=chat-1&cwd="+filepath.Join(workspace, "api"))
	client.input("echo cw''d=$(pwd)\n")
	client.until("cwd=" + filepath.Join(workspace, "api"))
}

func TestWorkingDirectoryStaysInsideTheWorkspace(t *testing.T) {
	workspace := realPath(t, t.TempDir())
	outside := realPath(t, t.TempDir())
	if err := os.Mkdir(filepath.Join(workspace, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		t.Fatal(err)
	}
	for requested, want := range map[string]string{
		filepath.Join(workspace, "api"):       filepath.Join(workspace, "api"),
		workspace:                             workspace,
		"":                                    workspace,
		"api":                                 workspace,
		outside:                               workspace,
		filepath.Join(workspace, "..", "etc"): workspace,
		filepath.Join(workspace, "api", "..", "..", filepath.Base(outside)): workspace,
		filepath.Join(workspace, "escape"):                                  workspace,
		filepath.Join(workspace, "missing"):                                 workspace,
		filepath.Join(workspace, "file"):                                    workspace,
		workspace + "-sibling":                                              workspace,
	} {
		if got := resolveWorkingDirectory(workspace, requested); got != want {
			t.Errorf("resolveWorkingDirectory(%q) = %q, want %q", requested, got, want)
		}
	}
}

func TestInvalidSessionNamesAreRejectedBeforeUpgrading(t *testing.T) {
	server, manager := startServer(t, sessionConfig{})
	for _, query := range []string{"", "session=", "session=a/b", "session=a%20b", "session=" + strings.Repeat("a", 129)} {
		_, response := dial(t, server, query, nil)
		if response.StatusCode != http.StatusBadRequest {
			t.Errorf("query %q: status = %d, want 400", query, response.StatusCode)
		}
	}
	if len(manager.sessions) != 0 {
		t.Fatalf("an invalid request started %d sessions", len(manager.sessions))
	}
}

func TestAnotherOriginCannotOpenAShell(t *testing.T) {
	server, manager := startServer(t, sessionConfig{})
	_, response := dial(t, server, "session=chat-1", map[string]string{"Origin": "https://evil.example"})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", response.StatusCode)
	}
	if len(manager.sessions) != 0 {
		t.Fatal("a cross-origin request started a session")
	}
	host := strings.TrimPrefix(server.URL, "http://")
	client, response := dial(t, server, "session=chat-1", map[string]string{"Origin": "http://" + host})
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("same-origin status = %d, want 101", response.StatusCode)
	}
	client.input("echo sa''me\n")
	client.until("same")
}

func TestSessionLimitRefusesNewShellsButNotReattachment(t *testing.T) {
	server, _ := startServer(t, sessionConfig{maxSessions: 1})
	first := connect(t, server, "session=chat-1")
	first.input("echo re''ady\n")
	first.until("ready")

	refused := connect(t, server, "session=chat-2")
	if code := refused.closeCode(); code != closeInternalError {
		t.Fatalf("close code = %d, want %d", code, closeInternalError)
	}
	again := connect(t, server, "session=chat-1")
	again.until("ready")
}

func TestMalformedMessagesDoNotEndTheSession(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	client := connect(t, server, "session=chat-1")
	client.send(opText, []byte("not json"))
	client.send(opText, []byte(`{"type":"resize","cols":0,"rows":-4}`))
	client.send(opText, []byte(`{"type":"resize","cols":100000,"rows":40}`))
	client.send(opText, []byte(`{"type":"unknown"}`))
	client.input("echo st''ill-here\n")
	client.until("still-here")
}

func TestResizeChangesTheTerminalSize(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	client := connect(t, server, "session=chat-1")
	client.send(opText, []byte(`{"type":"resize","cols":97,"rows":31}`))
	client.input("echo si''ze=$(stty size)\n")
	client.until("size=31 97")
}

func TestPageIsServedWithItsFramingPolicy(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	for host, want := range map[string]string{
		"terminal--demo.remote.example": "frame-ancestors https://remote.example",
		"localhost:8843":                "frame-ancestors 'self'",
		"plain.remote.example":          "frame-ancestors 'self'",
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Host = host
		recorder := httptest.NewRecorder()
		server.Config.Handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("host %q: status = %d", host, recorder.Code)
		}
		policy := recorder.Header().Get("Content-Security-Policy")
		if !strings.HasSuffix(policy, want) {
			t.Errorf("host %q: policy = %q, want suffix %q", host, policy, want)
		}
	}
	forwarded := httptest.NewRequest(http.MethodGet, "/", nil)
	forwarded.Host = "terminal--demo.remote.example"
	forwarded.Header.Set("X-Forwarded-Proto", "http")
	recorder := httptest.NewRecorder()
	server.Config.Handler.ServeHTTP(recorder, forwarded)
	if policy := recorder.Header().Get("Content-Security-Policy"); !strings.HasSuffix(policy, "frame-ancestors http://remote.example") {
		t.Errorf("forwarded http: policy = %q", policy)
	}
}

func TestEmbeddedAssetsAreServed(t *testing.T) {
	server, _ := startServer(t, sessionConfig{})
	for path, contentType := range map[string]string{
		"/":                        "text/html",
		"/terminal.js":             "text/javascript",
		"/reconnect.js":            "text/javascript",
		"/terminal.css":            "text/css",
		"/vendor/xterm.mjs":        "text/javascript",
		"/vendor/addon-fit.mjs":    "text/javascript",
		"/vendor/addon-search.mjs": "text/javascript",
		"/vendor/xterm.css":        "text/css",
	} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || len(body) == 0 {
			t.Errorf("%s: status = %d, %d bytes", path, response.StatusCode, len(body))
		}
		if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, contentType) {
			t.Errorf("%s: content type = %q, want %q", path, got, contentType)
		}
	}
	response, err := http.Post(server.URL+"/", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /: status = %d, want 405", response.StatusCode)
	}
}

func TestServeCommandValidatesItsArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"health"}, {"serve"}, {"serve", "--port", "0"}, {"serve", "--port", "70000"}, {"serve", "--port", "x"}} {
		if err := run(args); err == nil {
			t.Errorf("run(%q) succeeded", args)
		}
	}
}

func viewerCount(manager *sessionManager, id string) int {
	manager.mu.Lock()
	current := manager.sessions[id]
	manager.mu.Unlock()
	if current == nil {
		return -1
	}
	current.mu.Lock()
	defer current.mu.Unlock()
	return len(current.viewers)
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func realPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
