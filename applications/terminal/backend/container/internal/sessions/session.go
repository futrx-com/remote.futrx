//go:build linux

package sessions

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"futrx.local/catalog/applications/terminal/backend/container/internal/config"
)

// Size is a terminal window in character cells.
type Size struct {
	Cols, Rows int
}

func (s Size) valid() bool {
	return s.Cols >= 1 && s.Rows >= 1 && s.Cols <= config.MaxWindowCells && s.Rows <= config.MaxWindowCells
}

// Session is one shell and the recent output viewers read from it.
type Session struct {
	ID         string
	Generation uint64

	now         func() time.Time
	pollWait    time.Duration
	replayBytes int
	master      *os.File
	cmd         *exec.Cmd

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
		s.lastSeen = s.now()
		s.mu.Unlock()
	}()

	timer := time.NewTimer(s.pollWait)
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
	s.touch(s.now())
	_, err := s.master.Write(data)
	return err
}

func (s *Session) Resize(size Size) error {
	if !size.valid() {
		return ErrInvalidSize
	}
	s.touch(s.now())
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
	time.AfterFunc(config.HangupGrace, func() {
		// Once the shell is reaped its pid may belong to something else.
		if !s.Exited() {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
		}
	})
}

// pump copies shell output into the replay buffer until the shell exits.
func (s *Session) pump() {
	buffer := make([]byte, config.PTYReadBufferBytes)
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
	s.lastSeen = s.now()
	close(s.changed)
	s.changed = make(chan struct{})
	s.mu.Unlock()
}

func (s *Session) append(output []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.replay = append(s.replay, output...)
	if excess := len(s.replay) - s.replayBytes; excess > 0 {
		// Cut at a line boundary where one is near, so a replay does not open
		// in the middle of an escape sequence.
		if line := bytes.IndexByte(s.replay[excess:min(excess+config.ReplayLineSearchBytes, len(s.replay))], '\n'); line >= 0 {
			excess += line + 1
		}
		s.replay = append(s.replay[:0], s.replay[excess:]...)
		s.start += int64(excess)
	}
	close(s.changed)
	s.changed = make(chan struct{})
}
