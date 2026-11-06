//go:build linux

package sessions

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func testRegistry(t *testing.T, config Config) *Registry {
	t.Helper()
	if config.Command == nil {
		config.Command = func(string) *exec.Cmd {
			cmd := exec.Command("/bin/sh")
			cmd.Env = []string{"PATH=/usr/bin:/bin", "TERM=dumb", "PS1=$ "}
			return cmd
		}
	}
	if config.DetachedTTL == 0 {
		config.DetachedTTL = time.Minute
	}
	if config.MaxSessions == 0 {
		config.MaxSessions = 8
	}
	if config.ReplayBytes == 0 {
		config.ReplayBytes = 256 << 10
	}
	if config.PollWait == 0 {
		config.PollWait = 200 * time.Millisecond
	}
	if config.ReapInterval == 0 {
		config.ReapInterval = time.Hour
	}
	registry := NewRegistry(config)
	t.Cleanup(registry.Close)
	return registry
}

func attach(t *testing.T, registry *Registry, id string) *Session {
	t.Helper()
	current, err := registry.Attach(id, "", Size{Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	return current
}

// viewer reads a session as the browser does, from the replay onwards.
type viewer struct {
	t       *testing.T
	session *Session
	offset  int64
	seen    strings.Builder
	exited  bool
}

func watch(t *testing.T, current *Session) *viewer {
	return &viewer{t: t, session: current, offset: -1}
}

func (v *viewer) until(text string) {
	v.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(v.seen.String(), text) {
		if time.Now().After(deadline) {
			v.t.Fatalf("timed out waiting for %q in %q", text, v.seen.String())
		}
		v.read()
	}
}

func (v *viewer) read() Output {
	output := v.session.Read(context.Background(), v.offset)
	if output.Reset {
		v.seen.Reset()
	}
	v.seen.Write(output.Data)
	v.offset = output.Offset
	v.exited = v.exited || output.Exited
	return output
}

func (v *viewer) input(text string) {
	v.t.Helper()
	if err := v.session.Write([]byte(text)); err != nil {
		v.t.Fatal(err)
	}
}

func TestShellOutputReachesTheViewer(t *testing.T) {
	client := watch(t, attach(t, testRegistry(t, Config{}), "chat-1"))
	client.input("echo he''llo\n")
	client.until("hello")
}

func TestReattachingReturnsToTheSameShellWithItsOutput(t *testing.T) {
	registry := testRegistry(t, Config{})
	first := watch(t, attach(t, registry, "chat-1"))
	first.input("MARKER=kept; echo fi''rst\n")
	first.until("first")

	again := attach(t, registry, "chat-1")
	if again != first.session {
		t.Fatal("attaching again started another shell")
	}
	second := watch(t, again)
	// The replay shows what the earlier viewer had already seen, and the
	// variable proves it is the same shell rather than a new one.
	second.until("first")
	second.input("echo va''lue=$MARKER\n")
	second.until("value=kept")
	first.until("value=kept")
}

func TestSessionsAreSeparateShells(t *testing.T) {
	registry := testRegistry(t, Config{})
	first := watch(t, attach(t, registry, "chat-1"))
	first.input("MARKER=one; echo re''ady\n")
	first.until("ready")

	second := watch(t, attach(t, registry, "chat-2"))
	second.input("echo va''lue=[$MARKER]\n")
	second.until("value=[]")
}

func TestAnExitedShellIsReportedAndThenReplaced(t *testing.T) {
	registry := testRegistry(t, Config{})
	ended := attach(t, registry, "chat-1")
	client := watch(t, ended)
	client.input("echo by''e; exit\n")
	client.until("bye")
	deadline := time.Now().Add(10 * time.Second)
	for !client.exited {
		if time.Now().After(deadline) {
			t.Fatal("the exit was never reported")
		}
		client.read()
	}
	// A viewer of the ended shell can still find it to learn that it ended,
	// but cannot type into whatever replaces it.
	if _, ok := registry.Lookup("chat-1", ended.Generation); !ok {
		t.Fatal("the ended session was forgotten before its viewers saw it")
	}
	if err := ended.Write([]byte("echo lost\n")); err == nil {
		t.Fatal("wrote into an ended shell")
	}
	replacement := attach(t, registry, "chat-1")
	if replacement == ended || replacement.Generation == ended.Generation {
		t.Fatal("attaching after the exit did not start a new shell")
	}
	if _, ok := registry.Lookup("chat-1", ended.Generation); ok {
		t.Fatal("the old generation still resolves after replacement")
	}
}

func TestUnreadSessionIsEndedAfterTheGracePeriod(t *testing.T) {
	registry := testRegistry(t, Config{DetachedTTL: 50 * time.Millisecond, ReapInterval: 10 * time.Millisecond})
	current := attach(t, registry, "chat-1")
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, ok := registry.Lookup("chat-1", current.Generation); !ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the unread session was never collected")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestAWaitingReadKeepsTheSessionWatched(t *testing.T) {
	registry := testRegistry(t, Config{
		DetachedTTL: 50 * time.Millisecond, ReapInterval: 10 * time.Millisecond, PollWait: 400 * time.Millisecond,
	})
	client := watch(t, attach(t, registry, "chat-1"))
	client.input("MARKER=kept; echo re''ady\n")
	client.until("ready")
	// Each read waits longer than the grace period for output that never
	// comes; the shell must survive because someone is reading it.
	for range 3 {
		client.read()
	}
	client.input("echo va''lue=$MARKER\n")
	client.until("value=kept")
}

func TestSessionLimitRefusesNewShellsButNotReattachment(t *testing.T) {
	registry := testRegistry(t, Config{MaxSessions: 1})
	first := attach(t, registry, "chat-1")
	if _, err := registry.Attach("chat-2", "", Size{}); !errors.Is(err, ErrTooManySessions) {
		t.Fatalf("second shell: %v", err)
	}
	if again := attach(t, registry, "chat-1"); again != first {
		t.Fatal("reattaching was refused or started another shell")
	}
}

func TestResizeChangesTheTerminalSize(t *testing.T) {
	current := attach(t, testRegistry(t, Config{}), "chat-1")
	for _, size := range []Size{{0, 24}, {80, -4}, {100000, 40}} {
		if err := current.Resize(size); !errors.Is(err, ErrInvalidSize) {
			t.Fatalf("Resize(%+v) = %v", size, err)
		}
	}
	if err := current.Resize(Size{Cols: 97, Rows: 31}); err != nil {
		t.Fatal(err)
	}
	client := watch(t, current)
	client.input("echo si''ze=$(stty size)\n")
	client.until("size=31 97")
}

func TestNewShellsStartAtTheirInitialSizeAndDirectory(t *testing.T) {
	var requested []string
	registry := testRegistry(t, Config{Command: func(cwd string) *exec.Cmd {
		requested = append(requested, cwd)
		return exec.Command("/bin/sh")
	}})
	current, err := registry.Attach("chat-1", "/workspace/api", Size{Cols: 101, Rows: 33})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Attach("chat-1", "/workspace/other", Size{Cols: 10, Rows: 10}); err != nil {
		t.Fatal(err)
	}
	if len(requested) != 1 || requested[0] != "/workspace/api" {
		t.Fatalf("shells started for %q", requested)
	}
	client := watch(t, current)
	client.input("echo si''ze=$(stty size)\n")
	client.until("size=33 101")
}

func TestAViewerThatFellBehindIsResetToTheReplay(t *testing.T) {
	// Control output directly: an interactive shell may write its next prompt
	// after the command marker, so it cannot guarantee an idle stream here.
	current := &Session{
		now: time.Now, pollWait: time.Millisecond, replayBytes: 64,
		changed: make(chan struct{}),
	}
	current.append([]byte(strings.Repeat("line\n", 40)))
	behind := current.Read(context.Background(), 0)
	if !behind.Reset || len(behind.Data) > 64 || behind.Offset <= 64 {
		t.Fatalf("stale read = reset %v, %d bytes, offset %d", behind.Reset, len(behind.Data), behind.Offset)
	}
	if ahead := current.Read(context.Background(), behind.Offset+10); !ahead.Reset {
		t.Fatal("a read past the end was not reset")
	}
	// A caught-up viewer waits, then gets nothing new at the same offset.
	if idle := current.Read(context.Background(), behind.Offset); idle.Reset || len(idle.Data) != 0 || idle.Offset != behind.Offset {
		t.Fatalf("idle read = %+v", idle)
	}
}
