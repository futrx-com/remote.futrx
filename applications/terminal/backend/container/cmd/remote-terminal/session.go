//go:build linux

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// closeShellExited tells the page the session is over, so it must not
	// reconnect: doing so would start a new shell every time one exits.
	closeShellExited = 4000
	// closeClientTooSlow drops a viewer that stopped reading. It reconnects
	// and catches up from the replay buffer.
	closeClientTooSlow = 4001

	replayBytes = 256 << 10
	// How far past the cut to look for a line break when trimming the replay.
	replayLineSearchBytes = 4 << 10
	clientQueue           = 256
	hangupGrace           = 2 * time.Second
	maxWindowCells        = 1000
	ptyReadBufBytes       = 8192
)

var (
	sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

	errTooManySessions = errors.New("too many terminal sessions")
)

type sessionConfig struct {
	// workspace is the directory shells start in and may not leave by request.
	workspace string
	shell     []string
	env       []string
	// detachedTTL is how long a shell survives with nobody watching it.
	detachedTTL time.Duration
	maxSessions int
}

type sessionManager struct {
	config sessionConfig

	mu       sync.Mutex
	sessions map[string]*session
}

func newSessionManager(config sessionConfig) *sessionManager {
	return &sessionManager{config: config, sessions: map[string]*session{}}
}

// attach connects a viewer to the session named id, starting its shell in cwd
// when it does not exist yet. cwd is ignored for a session that is running.
func (m *sessionManager) attach(id, cwd string, viewer *viewer) (*session, error) {
	for {
		current, err := m.sessionFor(id, cwd)
		if err != nil {
			return nil, err
		}
		// The shell can exit between the lookup and the attach; start over
		// with a fresh one rather than hand back a dead session.
		if current.attach(viewer) {
			return current, nil
		}
	}
}

func (m *sessionManager) sessionFor(id, cwd string) (*session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.sessions[id]; ok {
		return current, nil
	}
	if len(m.sessions) >= m.config.maxSessions {
		return nil, errTooManySessions
	}
	cmd := exec.Command(m.config.shell[0], m.config.shell[1:]...)
	cmd.Env = m.config.env
	cmd.Dir = resolveWorkingDirectory(m.config.workspace, cwd)
	master, err := startInPTY(cmd)
	if err != nil {
		return nil, err
	}
	created := &session{
		id:      id,
		manager: m,
		master:  master,
		cmd:     cmd,
		viewers: map[*viewer]struct{}{},
	}
	m.sessions[id] = created
	go created.pump()
	return created, nil
}

func (m *sessionManager) forget(current *session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions[current.id] == current {
		delete(m.sessions, current.id)
	}
}

// resolveWorkingDirectory returns the directory a new shell starts in: the
// requested one when it is an existing directory inside the workspace, the
// workspace itself otherwise. Symlinks are resolved first, so a link pointing
// out of the workspace does not qualify.
func resolveWorkingDirectory(workspace, requested string) string {
	if !filepath.IsAbs(requested) {
		return workspace
	}
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return workspace
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(requested))
	if err != nil {
		return workspace
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return workspace
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return workspace
	}
	return resolved
}

// session is one shell and the viewers currently showing it.
type session struct {
	id      string
	manager *sessionManager
	master  *os.File
	cmd     *exec.Cmd

	mu      sync.Mutex
	viewers map[*viewer]struct{}
	// replay holds the most recent output, shown to a viewer that attaches
	// after it was produced.
	replay []byte
	exited bool
	reaper *time.Timer
}

func (s *session) attach(viewer *viewer) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exited {
		return false
	}
	if s.reaper != nil {
		s.reaper.Stop()
		s.reaper = nil
	}
	if len(s.replay) > 0 {
		viewer.output <- append([]byte(nil), s.replay...)
	}
	s.viewers[viewer] = struct{}{}
	return true
}

// detach removes a viewer. The shell keeps running; if nobody is left it is
// ended after the configured grace period unless someone reattaches.
func (s *session) detach(viewer *viewer) {
	s.mu.Lock()
	if _, ok := s.viewers[viewer]; ok {
		delete(s.viewers, viewer)
		if len(s.viewers) == 0 && !s.exited {
			s.reaper = time.AfterFunc(s.manager.config.detachedTTL, s.endIfUnwatched)
		}
	}
	s.mu.Unlock()
	viewer.finish(closeNormal, "")
}

func (s *session) endIfUnwatched() {
	s.mu.Lock()
	unwatched := len(s.viewers) == 0 && !s.exited
	s.mu.Unlock()
	if unwatched {
		s.end()
	}
}

// end hangs up the shell and everything it started in the terminal.
func (s *session) end() {
	pid := s.cmd.Process.Pid
	// The shell leads its own session, so its pid names the process group.
	_ = syscall.Kill(-pid, syscall.SIGHUP)
	time.AfterFunc(hangupGrace, func() {
		s.mu.Lock()
		exited := s.exited
		s.mu.Unlock()
		// Once the shell is reaped its pid may belong to something else.
		if !exited {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	})
}

func (s *session) write(data []byte) {
	_, _ = s.master.Write(data)
}

func (s *session) resize(cols, rows int) {
	if cols < 1 || rows < 1 || cols > maxWindowCells || rows > maxWindowCells {
		return
	}
	_ = setWindowSize(s.master, uint16(cols), uint16(rows))
}

// pump copies shell output to the replay buffer and every viewer until the
// shell exits, then closes the session.
func (s *session) pump() {
	buffer := make([]byte, ptyReadBufBytes)
	for {
		count, err := s.master.Read(buffer)
		if count > 0 {
			s.broadcast(buffer[:count])
		}
		if err != nil {
			break
		}
	}
	_ = s.master.Close()
	_ = s.cmd.Wait()

	s.mu.Lock()
	s.exited = true
	if s.reaper != nil {
		s.reaper.Stop()
		s.reaper = nil
	}
	viewers := s.viewers
	s.viewers = map[*viewer]struct{}{}
	s.mu.Unlock()

	s.manager.forget(s)
	for viewer := range viewers {
		viewer.finish(closeShellExited, "shell exited")
	}
}

func (s *session) broadcast(output []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replay = append(s.replay, output...)
	if excess := len(s.replay) - replayBytes; excess > 0 {
		// Cut at a line boundary where one is near, so a replay does not open
		// in the middle of an escape sequence.
		if line := bytes.IndexByte(s.replay[excess:min(excess+replayLineSearchBytes, len(s.replay))], '\n'); line >= 0 {
			excess += line + 1
		}
		s.replay = append(s.replay[:0], s.replay[excess:]...)
	}
	for viewer := range s.viewers {
		select {
		case viewer.output <- append([]byte(nil), output...):
		default:
			// Never let one stalled browser hold up the shell or the others.
			delete(s.viewers, viewer)
			viewer.finish(closeClientTooSlow, "client too slow")
		}
	}
	if len(s.viewers) == 0 && s.reaper == nil {
		s.reaper = time.AfterFunc(s.manager.config.detachedTTL, s.endIfUnwatched)
	}
}

// viewer is one WebSocket showing a session.
type viewer struct {
	conn   *wsConn
	output chan []byte

	finishOnce  sync.Once
	closeCode   uint16
	closeReason string
}

func newViewer(conn *wsConn) *viewer {
	created := &viewer{conn: conn, output: make(chan []byte, clientQueue)}
	go created.writeLoop()
	return created
}

func (v *viewer) writeLoop() {
	failed := false
	for output := range v.output {
		if failed {
			continue
		}
		if err := v.conn.writeMessage(opBinary, output); err != nil {
			failed = true
			// Unblocks the reader, which detaches this viewer.
			v.conn.close(closeInternalError, "")
		}
	}
	v.conn.close(v.closeCode, v.closeReason)
}

// finish flushes what is queued and closes the socket with code. Only the
// session calls it, after it has stopped sending to this viewer.
func (v *viewer) finish(code uint16, reason string) {
	v.finishOnce.Do(func() {
		v.closeCode, v.closeReason = code, reason
		close(v.output)
	})
}
