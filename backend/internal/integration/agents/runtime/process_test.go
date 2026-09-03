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
