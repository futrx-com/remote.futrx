package runtime

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const converseTestGrace = 200 * time.Millisecond

// askOneLine writes one request line and returns the first answer line.
func askOneLine(answer *string) func(io.Writer, io.Reader) error {
	return func(stdin io.Writer, stdout io.Reader) error {
		if _, err := io.WriteString(stdin, "request\n"); err != nil {
			return err
		}
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return io.ErrUnexpectedEOF
		}
		*answer = scanner.Text()
		return nil
	}
}

// readPID waits for a script to record a process ID in path.
func readPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) != "" {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatal(err)
			}
			return pid
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no process ID in %s", path)
	return 0
}

func requireGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d outlived the conversation", pid)
}

func TestConverseExchangesOneRequestAndLetsTheCLIExit(t *testing.T) {
	var answer string
	stderr, err := Converse(context.Background(),
		exec.Command("sh", "-c", `read line; echo "answer to $line"; echo note >&2`),
		converseTestGrace, askOneLine(&answer))
	if err != nil || answer != "answer to request" || stderr != "note" {
		t.Fatalf("answer = %q, stderr = %q, err = %v", answer, stderr, err)
	}
}

// An npm-installed CLI is a wrapper whose child does the work and shares its
// output. Ending the conversation must end the child too, not only the wrapper.
func TestConverseEndsAWrappedChildWhenTheContextEnds(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	wrapper := `sh -c 'echo $$ > "$1"; read line; exec sleep 30' child "$1" & wait`
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	started := time.Now()
	var answer string
	_, err := Converse(ctx, exec.Command("sh", "-c", wrapper, "wrapper", pidFile), converseTestGrace, askOneLine(&answer))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v; want the context's deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("Converse took %v after its deadline", elapsed)
	}
	requireGone(t, readPID(t, pidFile))
}

func TestConverseKillsACLIThatIgnoresTheEndOfItsInput(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "cli.pid")
	started := time.Now()
	var answer string
	_, err := Converse(context.Background(),
		exec.Command("sh", "-c", `echo $$ > "$1"; read line; echo answer; exec sleep 30`, "cli", pidFile),
		converseTestGrace, askOneLine(&answer))
	if err != nil || answer != "answer" {
		t.Fatalf("answer = %q, err = %v", answer, err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("Converse waited %v for a CLI that never exits", elapsed)
	}
	requireGone(t, readPID(t, pidFile))
}

// A descendant that left the process group can keep stdout open. The
// conversation still ends after grace instead of waiting for it.
func TestConverseReturnsWhileAnEscapedDescendantHoldsItsOutput(t *testing.T) {
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid is not installed")
	}
	pidFile := filepath.Join(t.TempDir(), "escaped.pid")
	script := `setsid sh -c 'echo $$ > "$1"; exec sleep 30' escaped "$1" & read line; echo answer`
	t.Cleanup(func() {
		if data, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})

	started := time.Now()
	var answer string
	_, err := Converse(context.Background(), exec.Command("sh", "-c", script, "cli", pidFile), converseTestGrace, askOneLine(&answer))
	if err != nil || answer != "answer" {
		t.Fatalf("answer = %q, err = %v", answer, err)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("Converse waited %v for an escaped descendant", elapsed)
	}
}

func TestConverseRefusesAnEndedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	_, err := Converse(ctx, exec.Command("sh", "-c", "exit 0"), converseTestGrace, func(io.Writer, io.Reader) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("err = %v, talked = %v", err, called)
	}
}

func TestLimitedBufferKeepsTheStartAndReportsEveryByte(t *testing.T) {
	buffer := &limitedBuffer{limit: 4}
	for _, chunk := range []string{"ab", "cdef", "gh"} {
		if n, err := buffer.Write([]byte(chunk)); n != len(chunk) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", chunk, n, err)
		}
	}
	if buffer.String() != "abcd" {
		t.Fatalf("kept %q", buffer.String())
	}
}
