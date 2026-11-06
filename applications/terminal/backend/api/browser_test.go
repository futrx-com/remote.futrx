//go:build linux

package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"futrx.local/catalog/applications/terminal/backend/containerio"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// TestTerminalBrowser drives the real drawer in Chromium against this API,
// served the way Remote forwards backend routes. The container command runs
// locally as both the service that owns the shells and the bridge, so only
// `lxc exec` itself is left out.
func TestTerminalBrowser(t *testing.T) {
	if os.Getenv("FUTRX_BROWSER_TEST") != "1" {
		t.Skip("set FUTRX_BROWSER_TEST=1 with Playwright installed")
	}
	_, source, _, _ := runtime.Caller(0)
	binary, socket, workspace := filepath.Join(t.TempDir(), "remote-terminal"), filepath.Join(t.TempDir(), "run", "terminal.sock"), t.TempDir()
	build := exec.Command("go", "build", "-o", binary, "./container/cmd/remote-terminal")
	build.Dir = filepath.Join(filepath.Dir(source), "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the container command: %v\n%s", err, output)
	}
	service := exec.Command(binary, "serve", "--socket", socket, "--workspace", workspace)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Process.Kill(); _ = service.Wait() })
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if _, err := os.Stat(socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the terminal service never opened its socket")
		}
	}
	var bridge *containerio.Bridge
	b := New(func(string) Relay {
		bridge = containerio.NewBridge(func() *exec.Cmd { return exec.Command(binary, "bridge", "--socket", socket) })
		return bridge
	})
	if err := b.Init(applications.Instance{Scope: "project", ContainerName: "project-demo"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bridge.Close() })
	ui := filepath.Join(filepath.Dir(source), "../../ui")

	mux := http.NewServeMux()
	mux.Handle("/ui/", http.StripPrefix("/ui/", http.FileServer(http.Dir(ui))))
	mux.HandleFunc("/api/applications/i1/backend/", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		response, err := b.Handle(applications.Request{
			Method: r.Method,
			Path:   strings.TrimPrefix(r.URL.Path, "/api/applications/i1/backend/"),
			Query:  r.URL.Query(),
			Body:   body,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		for name, values := range response.Headers {
			w.Header()[name] = values
		}
		w.WriteHeader(response.Status)
		_, _ = w.Write(response.Body)
	})
	server := httptest.NewServer(mux)
	// Ending the bridge first releases the drawer's last waiting read.
	defer func() { bridge.Close(); server.Close() }()

	command := exec.Command("node", "--test", filepath.Join(ui, "scripts/browser.test.mjs"))
	command.Env = append(os.Environ(), "FUTRX_TERMINAL_URL="+server.URL)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("browser: %v\n%s", err, output)
	}
	t.Log(string(output))
}
