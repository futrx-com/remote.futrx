package httptransport_test

import (
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFileManagerBrowser(t *testing.T) {
	if os.Getenv("FUTRX_BROWSER_TEST") != "1" {
		t.Skip("set FUTRX_BROWSER_TEST=1 with Playwright installed")
	}
	root, _ := setupWorkspace(t)
	server := httptest.NewServer(newHandler(root))
	defer server.Close()
	_, source, _, _ := runtime.Caller(0)
	script := filepath.Join(filepath.Dir(source), "../../../../../ui/tests/browser.test.mjs")
	command := exec.Command("node", "--test", script)
	command.Env = append(os.Environ(), "FUTRX_FILE_MANAGER_URL="+server.URL)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("browser: %v\n%s", err, output)
	}
	t.Log(string(output))
}
