//go:build linux

package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"futrx.local/catalog/applications/terminal/backend/container/internal/sessions"
)

func newTestServer(t *testing.T) (http.Handler, *[]string) {
	t.Helper()
	var cwds []string
	registry := sessions.NewRegistry(sessions.Config{
		Command: func(cwd string) *exec.Cmd {
			cwds = append(cwds, cwd)
			cmd := exec.Command("/bin/sh")
			cmd.Env = []string{"PATH=/usr/bin:/bin", "TERM=dumb", "PS1=$ "}
			return cmd
		},
		DetachedTTL:  time.Minute,
		MaxSessions:  8,
		ReplayBytes:  64 << 10,
		PollWait:     200 * time.Millisecond,
		ReapInterval: time.Hour,
	})
	t.Cleanup(registry.Close)
	return NewHandler(registry), &cwds
}

func call(t *testing.T, handler http.Handler, method, path string, query url.Values, body string) *httptest.ResponseRecorder {
	t.Helper()
	target := path
	if query != nil {
		target += "?" + query.Encode()
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(method, target, strings.NewReader(body)))
	return response
}

func open(t *testing.T, handler http.Handler, session, cwd string) attachResponse {
	t.Helper()
	body, _ := json.Marshal(attachRequest{Session: session, Cwd: cwd, Cols: 80, Rows: 24})
	response := call(t, handler, "POST", "/sessions", nil, string(body))
	var attached attachResponse
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &attached) != nil {
		t.Fatalf("attach = %d %s", response.Code, response.Body)
	}
	return attached
}

// readUntil polls output the way the drawer does until text appears.
func readUntil(t *testing.T, handler http.Handler, attached attachResponse, text string) {
	t.Helper()
	offset := int64(-1)
	var seen strings.Builder
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(seen.String(), text) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %q in %q", text, seen.String())
		}
		response := call(t, handler, "GET", "/output", url.Values{
			"session": {attached.Session}, "generation": {attached.Generation}, "offset": {fmt.Sprint(offset)},
		}, "")
		var output sessions.Output
		if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &output) != nil {
			t.Fatalf("output = %d %s", response.Code, response.Body)
		}
		if output.Reset {
			seen.Reset()
		}
		seen.Write(output.Data)
		offset = output.Offset
	}
}

func TestAShellIsOpenedTypedIntoAndRead(t *testing.T) {
	handler, cwds := newTestServer(t)
	attached := open(t, handler, "chat-1", "/workspace/api")
	if len(*cwds) != 1 || (*cwds)[0] != "/workspace/api" {
		t.Fatalf("shells started for %q", *cwds)
	}
	target := url.Values{"session": {attached.Session}, "generation": {attached.Generation}}
	if response := call(t, handler, "POST", "/input", target, "MARKER=kept; echo he''llo\n"); response.Code != 200 {
		t.Fatalf("input = %d %s", response.Code, response.Body)
	}
	readUntil(t, handler, attached, "hello")
	resize, _ := json.Marshal(resizeRequest{Session: attached.Session, Generation: attached.Generation, Cols: 97, Rows: 31})
	if response := call(t, handler, "POST", "/resize", nil, string(resize)); response.Code != 200 {
		t.Fatalf("resize = %d %s", response.Code, response.Body)
	}
	call(t, handler, "POST", "/input", target, "echo si''ze=$(stty size)\n")
	readUntil(t, handler, attached, "size=31 97")

	// Opening the chat again returns to the same shell.
	if again := open(t, handler, "chat-1", "/elsewhere"); again != attached || len(*cwds) != 1 {
		t.Fatalf("reopened as %+v after %q", again, *cwds)
	}
	call(t, handler, "POST", "/input", target, "echo va''lue=$MARKER\n")
	readUntil(t, handler, attached, "value=kept")
}

func TestRequestsAreValidated(t *testing.T) {
	handler, cwds := newTestServer(t)
	attached := open(t, handler, "chat-1", "relative/path")
	open(t, handler, "chat-2", "/workspace/"+strings.Repeat("a", 5000))
	if len(*cwds) != 2 || (*cwds)[0] != "" || (*cwds)[1] != "" {
		t.Fatalf("invalid directories reached the shell: %q", *cwds)
	}
	for _, test := range []struct {
		method, path string
		query        url.Values
		body         string
		status       int
	}{
		{"POST", "/sessions", nil, `{"session":"../x"}`, 400},
		{"POST", "/sessions", nil, `not json`, 400},
		{"GET", "/output", url.Values{"session": {"chat-1"}, "generation": {"x"}}, "", 400},
		{"GET", "/output", url.Values{"session": {"chat-1"}, "generation": {"999"}}, "", 404},
		{"GET", "/output", url.Values{"session": {"chat-9"}, "generation": {attached.Generation}}, "", 404},
		{"POST", "/input", url.Values{"session": {"chat-1"}, "generation": {"999"}}, "x", 404},
		{"POST", "/resize", nil, `{"session":"chat-1","generation":"` + attached.Generation + `","cols":0,"rows":5}`, 400},
		{"GET", "/sessions", nil, "", 405},
		{"GET", "/health", nil, "", 200},
	} {
		if response := call(t, handler, test.method, test.path, test.query, test.body); response.Code != test.status {
			t.Errorf("%s %s %v %s = %d %s, want %d", test.method, test.path, test.query, test.body,
				response.Code, response.Body, test.status)
		}
	}
}

func TestTheSocketIsRootOnlyAndReplacesAStaleOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run", "terminal.sock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	listener, err := Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	for name, want := range map[string]os.FileMode{filepath.Dir(path): 0o700, path: 0o600} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("%s mode = %v, %v; want %v", name, info.Mode().Perm(), err, want)
		}
	}
	if listener.Addr().Network() != "unix" {
		t.Fatalf("listening on %s", listener.Addr().Network())
	}
}
