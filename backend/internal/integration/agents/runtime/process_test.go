package runtime

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

type noOpParser struct{}

func (noOpParser) ParseLine([]byte) ([]agent.Event, error) { return nil, nil }

func TestRunProcessReturnsCapturedStderr(t *testing.T) {
	cmd := exec.Command("sh", "-c", "echo no rollout found for thread >&2; exit 1")
	err := RunProcess(context.Background(), cmd, noOpParser{}, nil, ProcessOptions{Name: "test"})
	if err == nil || !strings.Contains(ErrorStderr(err), "no rollout found") {
		t.Fatalf("error = %v, stderr = %q", err, ErrorStderr(err))
	}
	var processErr *ProcessError
	if !errors.As(err, &processErr) {
		t.Fatalf("error type = %T, want ProcessError", err)
	}
}

func TestRunProcessRetainsFinalDiagnosticAfterLongProgress(t *testing.T) {
	cmd := exec.Command("sh", "-c", `printf '%s\n' "$1" >&2; printf '%s\n' 'error: provider returned 307' >&2; exit 1`, "sh", strings.Repeat("progress ", 9000))
	err := RunProcess(context.Background(), cmd, noOpParser{}, nil, ProcessOptions{Name: "test"})
	stderr := ErrorStderr(err)
	if err == nil || !strings.HasSuffix(stderr, "error: provider returned 307\n") {
		t.Fatalf("final diagnostic missing: error=%v, captured bytes=%d", err, len(stderr))
	}
	if len(stderr) > 64<<10 {
		t.Fatalf("capture exceeded limit: %d bytes", len(stderr))
	}
}
