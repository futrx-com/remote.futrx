//go:build linux

// Package sessions keeps the project's shells running inside the container
// between browser requests.
// Each shell runs in a pseudo-terminal owned by this process; viewers read its
// output by offset, so any number of them, and a viewer that reconnects, see
// the same stream without the shell waiting for any of them.
package sessions

import (
	"errors"
	"os/exec"
	"regexp"
	"sync"
	"time"
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

// Registry owns chat-to-shell identity, session admission and collection.
type Registry struct {
	config Config
	now    func() time.Time

	mu         sync.Mutex
	sessions   map[string]*Session
	generation uint64
	stop       chan struct{}
	stopOnce   sync.Once
}

func NewRegistry(config Config) *Registry {
	m := &Registry{config: config, now: time.Now, sessions: map[string]*Session{}, stop: make(chan struct{})}
	go m.reapLoop()
	return m
}

// Attach returns the running shell named id, starting one in cwd when there
// is none. cwd and size apply only to a new shell.
func (m *Registry) Attach(id, cwd string, size Size) (*Session, error) {
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
		ID:          id,
		Generation:  m.generation,
		now:         m.now,
		pollWait:    m.config.PollWait,
		replayBytes: m.config.ReplayBytes,
		master:      master,
		cmd:         cmd,
		changed:     make(chan struct{}),
		lastSeen:    m.now(),
	}
	m.sessions[id] = created
	go created.pump()
	return created, nil
}

// Lookup returns the session named id when it is still the given generation,
// so a viewer of an ended shell never types into its replacement.
func (m *Registry) Lookup(id string, generation uint64) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.sessions[id]
	if !ok || current.Generation != generation {
		return nil, false
	}
	return current, true
}

// Close ends every shell and stops collecting sessions.
func (m *Registry) Close() {
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

func (m *Registry) live() int {
	count := 0
	for _, current := range m.sessions {
		if !current.Exited() {
			count++
		}
	}
	return count
}

func (m *Registry) reapLoop() {
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
func (m *Registry) reap() {
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
