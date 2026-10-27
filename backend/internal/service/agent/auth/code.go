package auth

// CodeService owns a single interactive CLI authorization-code login process.
// Providers supply command details, credential detection, and error wording via
// CodeConfig; this package owns only the shared PTY and session lifecycle.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

const (
	codeSubscriptionBuffer = 8
	defaultCodeOutputLimit = 500
	// codeOutputDrainWait bounds how long completion waits for the PTY
	// reader to collect the final output after the command exits. A
	// descendant still holding the terminal open must not stall completion.
	codeOutputDrainWait = time.Second
)

// CodeLoginState tracks an interactive authorization-code handshake.
type CodeLoginState struct {
	Active       bool   `json:"active"`
	AuthURL      string `json:"authUrl,omitempty"`
	AwaitingCode bool   `json:"awaitingCode,omitempty"`
	StartedAt    int64  `json:"startedAt,omitempty"`
	Completed    bool   `json:"completed,omitempty"`
	Error        string `json:"error,omitempty"`
}

// CodeStatus is the streamed credential and login snapshot.
type CodeStatus struct {
	Authenticated bool           `json:"authenticated"`
	Login         CodeLoginState `json:"login,omitempty"`
}

// CodeStartResult reports the authorization URL found in CLI output. Resumed
// is true when Start reused an already-running login session.
type CodeStartResult struct {
	URL     string
	Resumed bool
}

// CodeErrorFormatters preserves provider-facing error wording while keeping
// process management provider-neutral. Output arguments have already been
// truncated to CodeConfig.OutputLimit bytes.
type CodeErrorFormatters struct {
	PTYStart           func(error) error
	URLReadTimeout     func(time.Duration, string) error
	ExitBeforeURL      func(error, string) error
	WriteCode          func(error) error
	ExitTimeout        func(time.Duration, string) error
	Exit               func(error, string) error
	MissingCredentials func(string) error
}

// CodeConfig supplies all provider policy around the shared authorization-code
// lifecycle. Providers should set the three sentinel errors when callers rely
// on errors.Is or direct error comparison.
type CodeConfig struct {
	Command string
	Args    []string
	Env     func([]string) []string

	URLPattern     *regexp.Regexp
	LoginTimeout   time.Duration
	URLReadTimeout time.Duration
	ExitTimeout    time.Duration
	OutputLimit    int

	Authenticated func() bool
	NotFound      error
	CodeRequired  error
	NoSession     error
	Errors        CodeErrorFormatters

	// ResolveCompletion optionally takes over the outcome once the login
	// command exits after a submitted code. output is the cleaned tail of
	// the command's output. Returning handled=false keeps the default
	// exit-status and credential checks.
	ResolveCompletion func(exitErr error, output string) (handled bool, err error)
}

// CodeService owns one provider's interactive authorization-code process and
// streams status snapshots to subscribers.
type CodeService struct {
	config CodeConfig

	mu      sync.Mutex
	session *codeLoginSession
	state   CodeLoginState
	subs    map[chan CodeStatus]struct{}
}

type codeLoginSession struct {
	cmd       *exec.Cmd
	ptmx      *os.File
	startedAt time.Time
	cancel    context.CancelFunc
	done      chan struct{}
	// outputDone closes once the PTY reader has stopped.
	outputDone chan struct{}

	mu      sync.Mutex
	url     string
	output  strings.Builder
	exitErr error
	// cancelled marks a session stopped by Remote, whose exit is not a
	// login result.
	cancelled bool

	finishOnce sync.Once
	finishErr  error
}

// NewCodeService creates a provider-neutral authorization-code service.
func NewCodeService(config CodeConfig) *CodeService {
	config.Args = append([]string(nil), config.Args...)
	if config.OutputLimit <= 0 {
		config.OutputLimit = defaultCodeOutputLimit
	}
	return &CodeService{
		config: config,
		subs:   map[chan CodeStatus]struct{}{},
	}
}

func (s *CodeService) Authenticated() bool {
	if s.config.Authenticated == nil {
		return false
	}
	return s.config.Authenticated()
}

// IsInputError reports whether err represents invalid caller input or a
// missing interactive session, rather than an internal login failure.
func (s *CodeService) IsInputError(err error) bool {
	if err == nil {
		return false
	}
	return (s.config.CodeRequired != nil && errors.Is(err, s.config.CodeRequired)) ||
		(s.config.NoSession != nil && errors.Is(err, s.config.NoSession))
}

// Status returns the current credential and login snapshot.
func (s *CodeService) Status() CodeStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *CodeService) statusLocked() CodeStatus {
	return CodeStatus{Authenticated: s.Authenticated(), Login: s.state}
}

// Subscribe registers a status channel and immediately delivers the current
// snapshot. The returned function unsubscribes and closes the channel.
func (s *CodeService) Subscribe() (<-chan CodeStatus, func()) {
	ch := make(chan CodeStatus, codeSubscriptionBuffer)
	s.mu.Lock()
	if s.subs == nil {
		s.subs = map[chan CodeStatus]struct{}{}
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

// Broadcast pushes the current status to every subscriber.
func (s *CodeService) Broadcast() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.broadcastLocked()
}

func (s *CodeService) broadcastLocked() {
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

func (s *CodeService) setStateLocked(state CodeLoginState) {
	s.state = state
	s.broadcastLocked()
}

func (s *CodeService) setState(state CodeLoginState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setStateLocked(state)
}

// Start begins an interactive login, waits for its authorization URL, and
// resumes the existing session when one is already running.
func (s *CodeService) Start(ctx context.Context) (CodeStartResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	if s.session != nil {
		select {
		case <-s.session.done:
			s.session = nil
		default:
			result := CodeStartResult{URL: s.session.URL(), Resumed: true}
			s.mu.Unlock()
			return result, nil
		}
	}

	if _, err := exec.LookPath(s.config.Command); err != nil {
		notFound := s.config.NotFound
		if notFound == nil {
			notFound = fmt.Errorf("%s CLI not found on PATH", s.config.Command)
		}
		s.setStateLocked(CodeLoginState{Error: notFound.Error()})
		s.mu.Unlock()
		return CodeStartResult{}, notFound
	}

	loginCtx, cancel := context.WithTimeout(context.Background(), s.config.LoginTimeout)
	cmd := exec.CommandContext(loginCtx, s.config.Command, s.config.Args...)
	cmd.Env = s.commandEnv()

	ptmx, err := pty.Start(cmd)
	if err != nil {
		cancel()
		err = s.formatPTYStartError(err)
		s.setStateLocked(CodeLoginState{Error: err.Error()})
		s.mu.Unlock()
		return CodeStartResult{}, err
	}

	sess := &codeLoginSession{
		cmd:        cmd,
		ptmx:       ptmx,
		startedAt:  time.Now(),
		cancel:     cancel,
		done:       make(chan struct{}),
		outputDone: make(chan struct{}),
	}
	s.session = sess
	s.setStateLocked(CodeLoginState{Active: true, StartedAt: sess.startedAt.Unix()})
	s.mu.Unlock()

	urlFound := make(chan string, 1)
	go s.readLoginOutput(sess, urlFound)
	go waitForCodeLoginExit(sess)

	select {
	case url := <-urlFound:
		s.setState(CodeLoginState{
			Active:       true,
			AuthURL:      url,
			AwaitingCode: true,
			StartedAt:    sess.startedAt.Unix(),
		})
		// The CLI can also finish without a pasted code, for example when
		// the browser it opened completes the login through its local
		// callback. Resolve that exit as soon as it happens.
		go func() {
			<-sess.done
			if !sess.Cancelled() {
				_ = s.finish(sess)
			}
		}()
		return CodeStartResult{URL: url}, nil
	case <-time.After(s.config.URLReadTimeout):
		cancel()
		s.clear(sess)
		err := s.formatURLReadTimeoutError(s.config.URLReadTimeout, s.output(sess))
		s.setState(CodeLoginState{Error: err.Error()})
		return CodeStartResult{}, err
	case <-sess.done:
		s.clear(sess)
		err := s.formatExitBeforeURLError(sess.ExitErr(), s.output(sess))
		s.setState(CodeLoginState{Error: err.Error()})
		return CodeStartResult{}, err
	case <-ctx.Done():
		cancel()
		s.clear(sess)
		s.setState(CodeLoginState{Error: ctx.Err().Error()})
		return CodeStartResult{}, ctx.Err()
	}
}

// SubmitCode writes the pasted authorization code to the active CLI session
// and waits for credentials to be persisted.
func (s *CodeService) SubmitCode(ctx context.Context, code string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	code = strings.TrimSpace(code)
	if code == "" {
		if s.config.CodeRequired != nil {
			return s.config.CodeRequired
		}
		return errors.New("code is required")
	}

	sess := s.current()
	if sess == nil {
		s.mu.Lock()
		completed := s.state.Completed
		s.mu.Unlock()
		if completed {
			// The CLI already finished on its own; the code is not needed.
			return nil
		}
		if s.config.NoSession != nil {
			return s.config.NoSession
		}
		return errors.New("no login session in progress")
	}

	select {
	case <-sess.done:
		// The CLI exited before the code arrived; report how it ended.
		return s.finish(sess)
	default:
	}
	if _, err := io.WriteString(sess.ptmx, code+"\r"); err != nil {
		select {
		case <-sess.done:
			return s.finish(sess)
		case <-time.After(codeOutputDrainWait):
		}
		err = s.formatWriteCodeError(err)
		s.setState(CodeLoginState{Error: err.Error()})
		return err
	}

	select {
	case <-sess.done:
	case <-time.After(s.config.ExitTimeout):
		sess.MarkCancelled()
		sess.cancel()
		<-sess.done
		s.clear(sess)
		err := s.formatExitTimeoutError(s.config.ExitTimeout, s.output(sess))
		s.setState(CodeLoginState{Error: err.Error()})
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.finish(sess)
}

// finish resolves an exited session exactly once, whether the exit followed
// a pasted code or happened on its own, and returns the shared result.
func (s *CodeService) finish(sess *codeLoginSession) error {
	sess.finishOnce.Do(func() {
		sess.finishErr = s.resolveExit(sess)
	})
	return sess.finishErr
}

func (s *CodeService) resolveExit(sess *codeLoginSession) error {
	select {
	case <-sess.outputDone:
	case <-time.After(codeOutputDrainWait):
	}
	output := s.output(sess)
	exitErr := sess.ExitErr()
	s.clear(sess)

	if s.config.ResolveCompletion != nil {
		tail := OutputTail(sess.Output(), completionOutputLimit)
		if handled, err := s.config.ResolveCompletion(exitErr, tail); handled {
			if err != nil {
				s.setState(CodeLoginState{Error: err.Error()})
				return err
			}
			s.setState(CodeLoginState{Completed: true})
			return nil
		}
	}
	if exitErr != nil && !errors.Is(exitErr, context.Canceled) {
		err := s.formatExitError(exitErr, output)
		s.setState(CodeLoginState{Error: err.Error()})
		return err
	}
	if !s.Authenticated() {
		err := s.formatMissingCredentialsError(output)
		s.setState(CodeLoginState{Error: err.Error()})
		return err
	}
	s.setState(CodeLoginState{Completed: true})
	return nil
}

// Cancel stops the active session and clears its login state.
func (s *CodeService) Cancel(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	sess := s.current()
	if sess == nil {
		return nil
	}
	sess.MarkCancelled()
	sess.cancel()

	select {
	case <-sess.done:
		s.clear(sess)
		s.setState(CodeLoginState{})
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *CodeService) commandEnv() []string {
	environ := os.Environ()
	if s.config.Env == nil {
		return environ
	}
	return s.config.Env(environ)
}

func (s *CodeService) current() *codeLoginSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session
}

func (s *CodeService) clear(sess *codeLoginSession) {
	s.mu.Lock()
	if s.session == sess {
		s.session = nil
	}
	s.mu.Unlock()
}

func (s *CodeService) readLoginOutput(sess *codeLoginSession, urlFound chan<- string) {
	defer close(sess.outputDone)
	defer func() { _ = sess.ptmx.Close() }()

	reader := bufio.NewReader(sess.ptmx)
	buf := make([]byte, 4096)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			output := sess.AppendOutput(string(buf[:n]))
			if sess.URL() == "" && s.config.URLPattern != nil {
				if match := s.config.URLPattern.FindString(output); match != "" {
					sess.SetURL(match)
					select {
					case urlFound <- match:
					default:
					}
				}
			}
		}
		if readErr != nil {
			return
		}
	}
}

func waitForCodeLoginExit(sess *codeLoginSession) {
	sess.SetExitErr(sess.cmd.Wait())
	close(sess.done)
}

func (s *CodeService) output(sess *codeLoginSession) string {
	return truncateCodeOutput(sess.Output(), s.config.OutputLimit)
}

func (s *CodeService) formatPTYStartError(cause error) error {
	if format := s.config.Errors.PTYStart; format != nil {
		if err := format(cause); err != nil {
			return err
		}
	}
	return fmt.Errorf("start login PTY: %w", cause)
}

func (s *CodeService) formatURLReadTimeoutError(wait time.Duration, output string) error {
	if format := s.config.Errors.URLReadTimeout; format != nil {
		if err := format(wait, output); err != nil {
			return err
		}
	}
	return fmt.Errorf("did not see authorization URL within %s; output: %s", wait, output)
}

func (s *CodeService) formatExitBeforeURLError(cause error, output string) error {
	if format := s.config.Errors.ExitBeforeURL; format != nil {
		if err := format(cause, output); err != nil {
			return err
		}
	}
	if cause != nil {
		return fmt.Errorf("login command exited before printing authorization URL (%v); output: %s", cause, output)
	}
	return fmt.Errorf("login command exited before printing authorization URL; output: %s", output)
}

func (s *CodeService) formatWriteCodeError(cause error) error {
	if format := s.config.Errors.WriteCode; format != nil {
		if err := format(cause); err != nil {
			return err
		}
	}
	return fmt.Errorf("write authorization code to login command: %w", cause)
}

func (s *CodeService) formatExitTimeoutError(wait time.Duration, output string) error {
	if format := s.config.Errors.ExitTimeout; format != nil {
		if err := format(wait, output); err != nil {
			return err
		}
	}
	return fmt.Errorf("login command did not exit within %s after code paste; output: %s", wait, output)
}

func (s *CodeService) formatExitError(cause error, output string) error {
	if format := s.config.Errors.Exit; format != nil {
		if err := format(cause, output); err != nil {
			return err
		}
	}
	return fmt.Errorf("login command exited with error: %w; output: %s", cause, output)
}

func (s *CodeService) formatMissingCredentialsError(output string) error {
	if format := s.config.Errors.MissingCredentials; format != nil {
		if err := format(output); err != nil {
			return err
		}
	}
	return fmt.Errorf("login command exited cleanly but credentials were not found; output: %s", output)
}

func (s *codeLoginSession) AppendOutput(chunk string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.output.WriteString(chunk)
	return s.output.String()
}

func (s *codeLoginSession) Output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output.String()
}

func (s *codeLoginSession) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

func (s *codeLoginSession) SetURL(url string) {
	s.mu.Lock()
	s.url = url
	s.mu.Unlock()
}

func (s *codeLoginSession) MarkCancelled() {
	s.mu.Lock()
	s.cancelled = true
	s.mu.Unlock()
}

func (s *codeLoginSession) Cancelled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelled
}

func (s *codeLoginSession) ExitErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitErr
}

func (s *codeLoginSession) SetExitErr(err error) {
	s.mu.Lock()
	s.exitErr = err
	s.mu.Unlock()
}

func truncateCodeOutput(output string, limit int) string {
	if len(output) <= limit {
		return output
	}
	return output[:limit] + "..."
}
