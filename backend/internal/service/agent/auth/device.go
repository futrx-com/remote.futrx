package auth

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"
)

const subscriptionBuffer = 8

var ansiEscapeRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// DeviceState is the provider-neutral state of a CLI device-code login.
type DeviceState struct {
	Active          bool   `json:"active"`
	VerificationURI string `json:"verificationUri,omitempty"`
	UserCode        string `json:"userCode,omitempty"`
	StartedAt       int64  `json:"startedAt,omitempty"`
	ExpiresAt       int64  `json:"expiresAt,omitempty"`
	Completed       bool   `json:"completed,omitempty"`
	Error           string `json:"error,omitempty"`
}

// DeviceCompletion describes the provider-specific result after its login command
// exits and credentials have been inspected.
type DeviceCompletion struct {
	Completed bool
	Error     string
}

// DeviceStatusBuilder captures provider credential state before the login state is
// snapshotted. This preserves the ordering of the original provider services.
type DeviceStatusBuilder[S any] func(DeviceState) S

// DeviceConfig supplies the provider-specific policy around the shared device-login
// lifecycle.
type DeviceConfig[S any] struct {
	Command           string
	Args              []string
	Env               func([]string) []string
	NotFound          error
	StartErrorLabel   string
	ReadyTimeout      time.Duration
	LoginTimeout      time.Duration
	LoginTTL          time.Duration
	URLPattern        *regexp.Regexp
	CodePattern       *regexp.Regexp
	Authenticated     func() bool
	BuildStatus       func() DeviceStatusBuilder[S]
	ResolveCompletion func(error) DeviceCompletion
}

// DeviceService owns one provider's device-code login process and streams status
// snapshots to subscribers.
type DeviceService[S any] struct {
	config DeviceConfig[S]

	mu     sync.Mutex
	device DeviceState
	cancel context.CancelFunc
	subs   map[chan S]struct{}
}

func NewDeviceService[S any](config DeviceConfig[S]) *DeviceService[S] {
	return &DeviceService[S]{config: config, subs: map[chan S]struct{}{}}
}

func (s *DeviceService[S]) Authenticated() bool {
	if s.config.Authenticated == nil {
		return false
	}
	return s.config.Authenticated()
}

func (s *DeviceService[S]) Status() S {
	build := s.config.BuildStatus()
	return build(s.deviceSnapshot())
}

// LoginState returns the provider-neutral device handshake state without the
// provider-specific status wrapper used by compatibility routes.
func (s *DeviceService[S]) LoginState() DeviceState {
	return s.deviceSnapshot()
}

func (s *DeviceService[S]) Subscribe() (<-chan S, func()) {
	ch := make(chan S, subscriptionBuffer)
	s.mu.Lock()
	if s.subs == nil {
		s.subs = map[chan S]struct{}{}
	}
	s.subs[ch] = struct{}{}
	status := s.statusLocked()
	s.mu.Unlock()
	ch <- status

	cancel := func() {
		s.mu.Lock()
		if _, ok := s.subs[ch]; ok {
			delete(s.subs, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
	return ch, cancel
}

func (s *DeviceService[S]) StartDeviceLogin(ctx context.Context) (DeviceState, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, err := exec.LookPath(s.config.Command); err != nil {
		return DeviceState{}, s.config.NotFound
	}

	now := time.Now()

	s.mu.Lock()
	if s.device.Active && now.Unix() < s.device.ExpiresAt {
		state := s.device
		s.mu.Unlock()
		return state, nil
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}

	loginCtx, cancel := context.WithTimeout(context.Background(), s.config.LoginTimeout)
	cmd := exec.CommandContext(loginCtx, s.config.Command, s.config.Args...)
	cmd.Env = s.config.Env(os.Environ())

	reader, writer := io.Pipe()
	cmd.Stdout = writer
	cmd.Stderr = writer

	state := DeviceState{
		Active:    true,
		StartedAt: now.Unix(),
		ExpiresAt: now.Add(s.config.LoginTTL).Unix(),
	}
	s.device = state
	s.cancel = cancel

	ready := make(chan struct{})
	var readyOnce sync.Once
	markReady := func() {
		readyOnce.Do(func() {
			close(ready)
		})
	}

	if err := cmd.Start(); err != nil {
		cancel()
		_ = reader.Close()
		_ = writer.Close()
		s.device = DeviceState{Error: fmt.Sprintf("start %s: %v", s.config.StartErrorLabel, err)}
		s.cancel = nil
		state = s.device
		s.broadcastLocked()
		s.mu.Unlock()
		return state, err
	}

	go s.consumeDeviceLoginOutput(reader, markReady)
	done := make(chan struct{})
	go func() {
		err := cmd.Wait()
		_ = writer.Close()
		s.finishDeviceLogin(err)
		close(done)
	}()

	s.mu.Unlock()
	s.Broadcast()

	select {
	case <-ready:
		return s.deviceSnapshot(), nil
	case <-done:
		return s.deviceSnapshot(), nil
	case <-time.After(s.config.ReadyTimeout):
		return s.deviceSnapshot(), nil
	case <-ctx.Done():
		return s.deviceSnapshot(), ctx.Err()
	}
}

func (s *DeviceService[S]) deviceSnapshot() DeviceState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.device
}

func (s *DeviceService[S]) consumeDeviceLoginOutput(reader io.Reader, markReady func()) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := ansiEscapeRE.ReplaceAllString(scanner.Text(), "")
		changed := false
		s.mu.Lock()
		if url := s.config.URLPattern.FindString(line); url != "" {
			changed = changed || s.device.VerificationURI != url
			s.device.VerificationURI = url
		}
		if code := s.config.CodePattern.FindString(line); code != "" {
			changed = changed || s.device.UserCode != code
			s.device.UserCode = code
			if s.device.ExpiresAt == 0 {
				s.device.ExpiresAt = time.Now().Add(s.config.LoginTTL).Unix()
			}
		}
		ready := s.device.VerificationURI != "" && s.device.UserCode != ""
		if changed {
			s.broadcastLocked()
		}
		s.mu.Unlock()
		if ready {
			markReady()
		}
	}
}

func (s *DeviceService[S]) finishDeviceLogin(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}

	state := s.device
	state.Active = false
	completion := s.config.ResolveCompletion(err)
	state.Completed = completion.Completed
	state.Error = completion.Error
	s.device = state
	s.broadcastLocked()
}

func (s *DeviceService[S]) Broadcast() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.broadcastLocked()
}

func (s *DeviceService[S]) statusLocked() S {
	return s.config.BuildStatus()(s.device)
}

func (s *DeviceService[S]) broadcastLocked() {
	status := s.statusLocked()
	for ch := range s.subs {
		select {
		case ch <- status:
		default:
			delete(s.subs, ch)
			close(ch)
		}
	}
}
