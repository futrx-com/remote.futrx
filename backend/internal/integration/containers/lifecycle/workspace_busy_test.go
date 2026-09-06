package lifecycle

import (
	"os/exec"
	"strings"
	"testing"
)

// The workspace activity probe runs as an embedded Python program inside the
// project container, so nothing compiles it. Execute it on the test host to
// catch syntax or runtime errors; the verdict depends on the host and is not
// asserted beyond being one of the two expected words.
func TestWorkspaceBusyScriptRuns(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	out, err := exec.Command("python3", "-c", workspaceBusyScript).CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	verdict := strings.TrimSpace(string(out))
	if verdict != "busy" && verdict != "idle" {
		t.Fatalf("unexpected verdict %q", verdict)
	}
}
