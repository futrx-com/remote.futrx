package command

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

type notifiedOutput chan string

func (w notifiedOutput) Write(data []byte) (int, error) { w <- string(data); return len(data), nil }

func TestCommandPublishesOutputBeforeCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output := make(notifiedOutput, 4)
	done := make(chan struct {
		output string
		err    error
	}, 1)
	go func() {
		text, err := CaptureOutput(WithOutput(ctx, output), exec.CommandContext(ctx, "sh", "-c", "printf 'live output\\n'; exec sleep 60"))
		done <- struct {
			output string
			err    error
		}{text, err}
	}()
	select {
	case text := <-output:
		if !strings.Contains(text, "live output") {
			t.Fatalf("live output=%q", text)
		}
	case <-time.After(time.Second):
		t.Fatal("output was buffered until completion")
	}
	cancel()
	select {
	case result := <-done:
		if result.err == nil || !strings.Contains(result.output, "live output") {
			t.Fatalf("result=%+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("command did not stop on cancellation")
	}
}

func TestCaptureLimitRetainsTailWithoutChangingDefaultCapture(t *testing.T) {
	for _, limit := range []int{0, 1024} {
		ctx := context.Background()
		if limit > 0 {
			ctx = WithCaptureLimit(ctx, limit)
		}
		cmd := exec.CommandContext(ctx, "sh", "-c", "head -c 1048576 /dev/zero; printf LAST_BYTES")
		out, err := CaptureOutput(ctx, cmd)
		if err != nil {
			t.Fatal(err)
		}
		want := 1048576 + len("LAST_BYTES")
		if limit > 0 {
			want = limit
		}
		if len(out) != want || !strings.HasSuffix(out, "LAST_BYTES") {
			t.Fatalf("limit=%d: size=%d, want %d with final bytes", limit, len(out), want)
		}
	}
}

func TestTailBufferNeverGrowsPastLimitAcrossChunks(t *testing.T) {
	buffer := &tailBuffer{data: make([]byte, 0, 8), limit: 8}
	for _, chunk := range []string{"abc", "defg", "hijkl", "0123456789", "!"} {
		n, err := buffer.Write([]byte(chunk))
		if err != nil || n != len(chunk) || len(buffer.data) > 8 || cap(buffer.data) > 8 {
			t.Fatalf("unbounded buffer after %q", chunk)
		}
	}
	if got := buffer.String(); got != "3456789!" {
		t.Fatalf("tail = %q", got)
	}
}
