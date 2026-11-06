//go:build linux

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"futrx.local/catalog/applications/terminal/backend/containerio"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

var (
	buildOnce   sync.Once
	builtBinary string
	buildErr    error
)

// containerCommand builds backend/container's remote-terminal for this run.
func containerCommand(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		_, source, _, _ := runtime.Caller(0)
		directory, err := os.MkdirTemp("", "remote-terminal-test-")
		if err != nil {
			buildErr = err
			return
		}
		builtBinary = filepath.Join(directory, "remote-terminal")
		build := exec.Command("go", "build", "-o", builtBinary, "./container/cmd/remote-terminal")
		build.Dir = filepath.Join(filepath.Dir(source), "..")
		if output, err := build.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("%v\n%s", err, output)
		}
	})
	if buildErr != nil {
		t.Fatalf("build the container command: %v", buildErr)
	}
	return builtBinary
}

// startService runs `remote-terminal serve` as the container's systemd unit would.
func startService(t *testing.T) (binary, socket string) {
	t.Helper()
	binary = containerCommand(t)
	socket = filepath.Join(t.TempDir(), "run", "terminal.sock")
	service := exec.Command(binary, "serve", "--socket", socket, "--workspace", t.TempDir())
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Process.Kill(); _ = service.Wait() })
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(socket); err == nil {
			return binary, socket
		}
		if time.Now().After(deadline) {
			t.Fatal("the terminal service never opened its socket")
		}
	}
}

// newBackend is one run of this host backend, relaying through its own bridge.
func newBackend(t *testing.T, binary, socket string) (*API, *containerio.Bridge) {
	t.Helper()
	var bridge *containerio.Bridge
	b := New(func(container string) Relay {
		if container != "project-demo" {
			t.Fatalf("bridge opened into %q", container)
		}
		bridge = containerio.NewBridge(func() *exec.Cmd { return exec.Command(binary, "bridge", "--socket", socket) })
		return bridge
	})
	if err := b.Init(applications.Instance{Scope: "project", ContainerName: "project-demo"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bridge.Close)
	return b, bridge
}

func call(t *testing.T, b *API, method, path string, query url.Values, body string) applications.Response {
	t.Helper()
	response, err := b.Handle(applications.Request{Method: method, Path: path, Query: query, Body: []byte(body)})
	if err != nil {
		t.Fatal(err)
	}
	return response
}

type attached struct {
	Session    string `json:"session"`
	Generation string `json:"generation"`
}

func open(t *testing.T, b *API, session string) attached {
	t.Helper()
	response := call(t, b, "POST", "sessions", nil, `{"session":"`+session+`","cols":80,"rows":24}`)
	var shell attached
	if response.Status != 200 || json.Unmarshal(response.Body, &shell) != nil {
		t.Fatalf("attach = %d %s", response.Status, response.Body)
	}
	return shell
}

func readUntil(t *testing.T, b *API, shell attached, text string) {
	t.Helper()
	offset := "-1"
	var seen strings.Builder
	deadline := time.Now().Add(15 * time.Second)
	for !strings.Contains(seen.String(), text) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %q in %q", text, seen.String())
		}
		response := call(t, b, "GET", "output", url.Values{
			"session": {shell.Session}, "generation": {shell.Generation}, "offset": {offset},
		}, "")
		var output struct {
			Offset int64  `json:"offset"`
			Data   []byte `json:"data"`
			Reset  bool   `json:"reset"`
		}
		if response.Status != 200 || json.Unmarshal(response.Body, &output) != nil {
			t.Fatalf("output = %d %s", response.Status, response.Body)
		}
		if output.Reset {
			seen.Reset()
		}
		seen.Write(output.Data)
		offset = fmt.Sprint(output.Offset)
	}
}

func typeLine(t *testing.T, b *API, shell attached, line string) {
	t.Helper()
	target := url.Values{"session": {shell.Session}, "generation": {shell.Generation}}
	if response := call(t, b, "POST", "input", target, line+"\n"); response.Status != 200 {
		t.Fatalf("input = %d %s", response.Status, response.Body)
	}
}

func TestShellsOutliveTheBridgeAndTheBackend(t *testing.T) {
	binary, socket := startService(t)
	first, bridge := newBackend(t, binary, socket)
	shell := open(t, first, "chat-1")
	typeLine(t, first, shell, "MARKER=kept; echo fi''rst")
	readUntil(t, first, shell, "first")

	// The bridge into the container dies; the next call starts another.
	bridge.Close()
	typeLine(t, first, shell, "echo after-br''idge")
	readUntil(t, first, shell, "after-bridge")

	// A new backend process, as after Remote restarts, finds the same shell.
	second, _ := newBackend(t, binary, socket)
	if again := open(t, second, "chat-1"); again != shell {
		t.Fatalf("reopened as %+v, want %+v", again, shell)
	}
	readUntil(t, second, shell, "first")
	typeLine(t, second, shell, "echo va''lue=$MARKER")
	readUntil(t, second, shell, "value=kept")
}

func TestServiceAnswersPassThrough(t *testing.T) {
	binary, socket := startService(t)
	b, _ := newBackend(t, binary, socket)
	for _, test := range []struct {
		method, path string
		query        url.Values
		body         string
		status       int
	}{
		{"POST", "sessions", nil, `{"session":"../x"}`, 400},
		{"GET", "output", url.Values{"session": {"chat-1"}, "generation": {"999"}}, "", 404},
		{"GET", "sessions", nil, "", 405},
	} {
		response := call(t, b, test.method, test.path, test.query, test.body)
		if response.Status != test.status {
			t.Errorf("%s %s = %d %s, want %d", test.method, test.path, response.Status, response.Body, test.status)
		}
		if test.status != 405 && response.Headers["Content-Type"][0] != "application/json" {
			t.Errorf("%s %s content type = %v", test.method, test.path, response.Headers)
		}
	}
}

type failingRelay struct{}

func (failingRelay) Do(context.Context, containerio.Request) (containerio.Response, error) {
	return containerio.Response{}, fmt.Errorf("container is stopped")
}

func TestAnUnreachableContainerIsReported(t *testing.T) {
	b := New(func(string) Relay { return failingRelay{} })
	if response := call(t, b, "POST", "sessions", nil, `{}`); response.Status != 409 {
		t.Fatalf("without a container = %d", response.Status)
	}
	if err := b.Init(applications.Instance{ContainerName: "project-demo"}); err != nil {
		t.Fatal(err)
	}
	response := call(t, b, "POST", "sessions", nil, `{"session":"chat-1"}`)
	if response.Status != 502 || !strings.Contains(string(response.Body), "container is stopped") {
		t.Fatalf("unreachable = %d %s", response.Status, response.Body)
	}
}
