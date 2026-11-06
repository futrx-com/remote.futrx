//go:build linux

// Package sessions keeps the project's shells running inside the container
// between browser requests.
// Each shell runs in a pseudo-terminal owned by this process; viewers read its
// output by offset, so any number of them, and a viewer that reconnects, see
// the same stream without the shell waiting for any of them.
package sessions

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"syscall"
	"time"
)

const (
	// How far past the cut to look for a line break when trimming the replay.
	replayLineSearchBytes = 4 << 10
	hangupGrace           = 2 * time.Second
	maxWindowCells        = 1000
	ptyReadBufBytes       = 8192
)

var (
	IDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

	ErrTooManySessions = errors.New("too many terminal sessions")
	ErrInvalidSize     = errors.New("invalid terminal size")
)

// Config describes how shells start and how long they are kept.
type Config struct {
	// Command builds the process for a new shell that should start in cwd.
	Command     func(cwd string) *exec.Cmd
	DetachedTTL time.Duration
	MaxSessions int
	ReplayBytes int
	PollWait    time.Duration
	// ReapInterval is how often unwatched and exited sessions are collected.
	ReapInterval time.Duration
}

// Size is a terminal window in character cells.
type Size struct {
	Cols, Rows int
}

func (s Size) valid() bool {
	return s.Cols >= 1 && s.Rows >= 1 && s.Cols <= maxWindowCells && s.Rows <= maxWindowCells
}

type Manager struct {
	config Config
	now    func() time.Time

	mu         sync.Mutex
	sessions   map[string]*Session
	generation uint64
	stop       chan struct{}
	stopOnce   sync.Once
}

func NewManager(config Config) *Manager {
	m := &Manager{config: config, now: time.Now, sessions: map[string]*Session{}, stop: make(chan struct{})}
	go m.reapLoop()
	return m
}

// Attach returns the running shell named id, starting one in cwd when there
// is none. cwd and size apply only to a new shell.
func (m *Manager) Attach(id, cwd string, size Size) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, ok := m.sessions[id]; ok && !current.Exited() {
		current.touch(m.now())
		return current, nil
	}
	if m.live() >= m.config.MaxSessions {
		return nil, ErrTooManySessions
	}
	cmd := m.config.Command(cwd)
	master, err := startInPTY(cmd)
	if err != nil {
		return nil, err
	}
	if size.valid() {
		_ = setWindowSize(master, uint16(size.Cols), uint16(size.Rows))
	}
	m.generation++
	created := &Session{
		ID:         id,
		Generation: m.generation,
		manager:    m,
		master:     master,
		cmd:        cmd,
		changed:    make(chan struct{}),
		lastSeen:   m.now(),
	}
	m.sessions[id] = created
	go created.pump()
	return created, nil
}

// Lookup returns the session named id when it is still the given generation,
// so a viewer of an ended shell never types into its replacement.
func (m *Manager) Lookup(id string, generation uint64) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.sessions[id]
	if !ok || current.Generation != generation {
		return nil, false
	}
	return current, true
}

// Close ends every shell and stops collecting sessions.
func (m *Manager) Close() {
	m.stopOnce.Do(func() { close(m.stop) })
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, current := range m.sessions {
		sessions = append(sessions, current)
	}
	m.mu.Unlock()
	for _, current := range sessions {
		current.end()
	}
}

func (m *Manager) live() int {
	count := 0
	for _, current := range m.sessions {
		if !current.Exited() {
			count++
		}
	}
	return count
}

func (m *Manager) reapLoop() {
	ticker := time.NewTicker(m.config.ReapInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.reap()
		case <-m.stop:
			return
		}
	}
}

// reap ends shells nobody has read for DetachedTTL and forgets exited ones
// once their last viewers have had the same time to notice.
func (m *Manager) reap() {
	now := m.now()
	var unwatched []*Session
	m.mu.Lock()
	for id, current := range m.sessions {
		idle, exited := current.idleSince(now)
		if idle < m.config.DetachedTTL {
			continue
		}
		if exited {
			delete(m.sessions, id)
		} else {
			unwatched = append(unwatched, current)
		}
	}
	m.mu.Unlock()
	for _, current := range unwatched {
		current.end()
	}
}

// Session is one shell and the recent output viewers read from it.
type Session struct {
	ID         string
	Generation uint64

	manager *Manager
	master  *os.File
	cmd     *exec.Cmd

	mu sync.Mutex
	// replay holds the most recent output; start is the stream offset of its
	// first byte.
	replay  []byte
	start   int64
	exited  bool
	changed chan struct{}
	// readers counts output reads in progress. A session being read is
	// watched, however long ago the last read began.
	readers  int
	lastSeen time.Time
}

// Output is what a read returns: the stream from the requested offset up to
// Offset. Reset means the requested offset was no longer available and Data
// starts at the oldest retained output instead.
type Output struct {
	Offset int64  `json:"offset"`
	Data   []byte `json:"data"`
	Reset  bool   `json:"reset"`
	Exited bool   `json:"exited"`
}

// Read returns output after offset, waiting up to PollWait for some. A
// negative offset asks for the whole replay, as a new viewer does.
func (s *Session) Read(ctx context.Context, offset int64) Output {
	s.mu.Lock()
	s.readers++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.readers--
		s.lastSeen = s.manager.now()
		s.mu.Unlock()
	}()

	timer := time.NewTimer(s.manager.config.PollWait)
	defer timer.Stop()
	for {
		s.mu.Lock()
		end := s.start + int64(len(s.replay))
		switch {
		case offset < s.start || offset > end:
			output := Output{Offset: end, Data: append([]byte(nil), s.replay...), Reset: true, Exited: s.exited}
			s.mu.Unlock()
			return output
		case offset < end:
			output := Output{Offset: end, Data: append([]byte(nil), s.replay[offset-s.start:]...)}
			s.mu.Unlock()
			return output
		case s.exited:
			s.mu.Unlock()
			return Output{Offset: end, Exited: true}
		}
		changed := s.changed
		s.mu.Unlock()
		select {
		case <-changed:
		case <-timer.C:
			return Output{Offset: offset}
		case <-ctx.Done():
			return Output{Offset: offset}
		}
	}
}

func (s *Session) Write(data []byte) error {
	s.touch(s.manager.now())
	_, err := s.master.Write(data)
	return err
}

func (s *Session) Resize(size Size) error {
	if !size.valid() {
		return ErrInvalidSize
	}
	s.touch(s.manager.now())
	return setWindowSize(s.master, uint16(size.Cols), uint16(size.Rows))
}

func (s *Session) Exited() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exited
}

func (s *Session) touch(now time.Time) {
	s.mu.Lock()
	s.lastSeen = now
	s.mu.Unlock()
}

// idleSince reports how long nobody has read the session, zero while a read
// is in progress.
func (s *Session) idleSince(now time.Time) (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.readers > 0 {
		return 0, s.exited
	}
	return now.Sub(s.lastSeen), s.exited
}

// end hangs up the shell and everything it started in the terminal.
func (s *Session) end() {
	pid := s.cmd.Process.Pid
	// The shell leads its own session, so its pid names the process group.
	_ = syscall.Kill(-pid, syscall.SIGHUP)
	time.AfterFunc(hangupGrace, func() {
		// Once the shell is reaped its pid may belong to something else.
		if !s.Exited() {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	})
}

// pump copies shell output into the replay buffer until the shell exits.
func (s *Session) pump() {
	buffer := make([]byte, ptyReadBufBytes)
	for {
		count, err := s.master.Read(buffer)
		if count > 0 {
			s.append(buffer[:count])
		}
		if err != nil {
			break
		}
	}
	_ = s.master.Close()
	_ = s.cmd.Wait()

	s.mu.Lock()
	s.exited = true
	// An exited session is kept for its viewers; idle time starts now.
	s.lastSeen = s.manager.now()
	close(s.changed)
	s.changed = make(chan struct{})
	s.mu.Unlock()
}

func (s *Session) append(output []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replay = append(s.replay, output...)
	if excess := len(s.replay) - s.manager.config.ReplayBytes; excess > 0 {
		// Cut at a line boundary where one is near, so a replay does not open
		// in the middle of an escape sequence.
		if line := bytes.IndexByte(s.replay[excess:min(excess+replayLineSearchBytes, len(s.replay))], '\n'); line >= 0 {
			excess += line + 1
		}
		s.replay = append(s.replay[:0], s.replay[excess:]...)
		s.start += int64(excess)
	}
	close(s.changed)
	s.changed = make(chan struct{})
}
