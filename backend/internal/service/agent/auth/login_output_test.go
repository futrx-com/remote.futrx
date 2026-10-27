package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOutputTailCleansTerminalOutput(t *testing.T) {
	cases := []struct {
		name, output string
		limit        int
		want         string
	}{
		{"colors", "\x1b[94mABCD-12345\x1b[0m\n", 100, "ABCD-12345"},
		{"hyperlink", "visit: \x1b]8;;https://x.test/a\x07https://x.test/a\x1b]8;;\x07 now", 100, "visit: https://x.test/a now"},
		{"hyperlink with ST", "\x1b]8;;https://x.test\x1b\\link\x1b]8;;\x1b\\", 100, "link"},
		{"cursor movement", "\x1b[2K\x1b[1GLogin successful.\r\n", 100, "Login successful."},
		{"whitespace", "  first\r\n\n\tsecond  ", 100, "first second"},
		{"keeps the tail", "banner that is long\nfinal error", 11, "...final error"},
		{"rune boundary", "ééééé", 3, "...é"},
		{"no limit", "a b", 0, "a b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OutputTail(tc.output, tc.limit); got != tc.want {
				t.Fatalf("OutputTail(%q, %d) = %q, want %q", tc.output, tc.limit, got, tc.want)
			}
		})
	}
}

func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "login")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

type recordedCompletion struct {
	mu      sync.Mutex
	calls   int
	err     error
	output  string
	outputs []string
}

func (r *recordedCompletion) resolve(err error, output string) DeviceCompletion {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.err, r.output = err, output
	r.outputs = append(r.outputs, output)
	return DeviceCompletion{Completed: err == nil}
}

func (r *recordedCompletion) snapshot() (int, error, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.err, r.output
}

func testDeviceService(command string, ttl time.Duration, completion *recordedCompletion) *DeviceService[DeviceState] {
	return NewDeviceService(DeviceConfig[DeviceState]{
		Command:         command,
		Env:             func(base []string) []string { return base },
		StartErrorLabel: "test login",
		ReadyTimeout:    2 * time.Second,
		LoginTimeout:    10 * time.Second,
		LoginTTL:        ttl,
		URLPattern:      regexp.MustCompile(`https://device\.test/[a-z]+`),
		CodePattern:     regexp.MustCompile(`[A-Z0-9]{4}-[A-Z0-9]{5}`),
		BuildStatus: func() DeviceStatusBuilder[DeviceState] {
			return func(state DeviceState) DeviceState { return state }
		},
		ResolveCompletion: completion.resolve,
	})
}

func waitForDeviceLogin(t *testing.T, service *DeviceService[DeviceState]) DeviceState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for service.LoginState().Active {
		if time.Now().After(deadline) {
			t.Fatal("device login did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return service.LoginState()
}

func TestDeviceLoginPassesFinalOutputToCompletion(t *testing.T) {
	script := writeScript(t, `
printf '\033[94mhttps://device.test/login\033[0m\n'
printf 'ABCD-12345\n'
printf 'Error: \033[31mdevice code expired\033[0m'
`)
	completion := &recordedCompletion{}
	service := testDeviceService(script, time.Minute, completion)

	state, err := service.StartDeviceLogin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if state.VerificationURI != "https://device.test/login" || state.UserCode != "ABCD-12345" {
		t.Fatalf("started state = %#v", state)
	}
	waitForDeviceLogin(t, service)
	calls, exitErr, output := completion.snapshot()
	if calls != 1 || exitErr != nil {
		t.Fatalf("completion calls = %d, err = %v", calls, exitErr)
	}
	// The last line has no trailing newline; it must still be delivered.
	if !strings.HasSuffix(output, "Error: device code expired") || strings.Contains(output, "\x1b") {
		t.Fatalf("completion output = %q", output)
	}
}

func TestDeviceLoginReplacedProcessDoesNotResolveTheNewLogin(t *testing.T) {
	gate := filepath.Join(t.TempDir(), "gate")
	// The gate is claimed before the prompt is printed, so only the first
	// process can take the long-running branch.
	script := writeScript(t, `
if mkdir "`+gate+`" 2>/dev/null; then
  printf 'https://device.test/login\n'
  printf 'ABCD-12345\n'
  printf 'first process\n'
  exec sleep 30
fi
printf 'https://device.test/login\n'
printf 'ABCD-12345\n'
printf 'second process\n'
`)
	completion := &recordedCompletion{}
	// A zero TTL makes the first login immediately replaceable.
	service := testDeviceService(script, 0, completion)
	if _, err := service.StartDeviceLogin(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartDeviceLogin(context.Background()); err != nil {
		t.Fatal(err)
	}
	state := waitForDeviceLogin(t, service)
	// Give the killed first process time to report, if it wrongly would.
	time.Sleep(200 * time.Millisecond)

	calls, exitErr, output := completion.snapshot()
	if calls != 1 || exitErr != nil || !state.Completed {
		t.Fatalf("completion calls = %d, err = %v, state = %#v, outputs = %q", calls, exitErr, state, completion.outputs)
	}
	if !strings.Contains(output, "second process") || strings.Contains(output, "first process") {
		t.Fatalf("completion output = %q", output)
	}
}

func TestCodeLoginPassesFinalOutputToCompletion(t *testing.T) {
	script := writeScript(t, `
printf 'visit: https://code.test/authorize?x=1\n'
printf 'Paste code here if prompted > '
read code
printf '\033[32mLogin successful for %s.\033[0m\n' "$code"
`)
	var (
		mu       sync.Mutex
		gotErr   error
		gotOut   string
		resolved bool
	)
	service := NewCodeService(CodeConfig{
		Command:        script,
		URLPattern:     regexp.MustCompile(`https://code\.test/authorize\?[^\s]+`),
		LoginTimeout:   10 * time.Second,
		URLReadTimeout: 5 * time.Second,
		ExitTimeout:    5 * time.Second,
		Authenticated:  func() bool { return false },
		ResolveCompletion: func(exitErr error, output string) (bool, error) {
			mu.Lock()
			defer mu.Unlock()
			resolved, gotErr, gotOut = true, exitErr, output
			return true, nil
		},
	})
	if _, err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.SubmitCode(context.Background(), "pasted"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !resolved || gotErr != nil {
		t.Fatalf("resolved = %v, err = %v", resolved, gotErr)
	}
	if !strings.HasSuffix(gotOut, "Login successful for pasted.") || strings.Contains(gotOut, "\x1b") {
		t.Fatalf("completion output = %q", gotOut)
	}
	if state := service.Status().Login; !state.Completed {
		t.Fatalf("login state = %#v", state)
	}
}

func TestNormalizeLoginLabelAllowsBlankLabelOnlyForReconnect(t *testing.T) {
	if label, err := normalizeLoginLabel("  ", "account-id"); err != nil || label != "" {
		t.Fatalf("reconnect label = %q, %v", label, err)
	}
	if _, err := normalizeLoginLabel("  ", ""); err != ErrAccountLabelRequired {
		t.Fatalf("new account blank label error = %v", err)
	}
	if label, err := normalizeLoginLabel("  Company ", "account-id"); err != nil || label != "Company" {
		t.Fatalf("reconnect with label = %q, %v", label, err)
	}
}

type recordedCodeCompletion struct {
	mu     sync.Mutex
	calls  int
	err    error
	output string
}

func (r *recordedCodeCompletion) resolve(exitErr error, output string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.err, r.output = exitErr, output
	if exitErr != nil {
		return true, errors.New("login failed: " + output)
	}
	return true, nil
}

func (r *recordedCodeCompletion) snapshot() (int, error, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.err, r.output
}

func testCodeService(script string, completion *recordedCodeCompletion) *CodeService {
	return NewCodeService(CodeConfig{
		Command:           script,
		URLPattern:        regexp.MustCompile(`https://code\.test/authorize\?[^\s]+`),
		LoginTimeout:      10 * time.Second,
		URLReadTimeout:    5 * time.Second,
		ExitTimeout:       5 * time.Second,
		Authenticated:     func() bool { return false },
		ResolveCompletion: completion.resolve,
	})
}

func waitForCodeLogin(t *testing.T, service *CodeService) CodeLoginState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for service.Status().Login.Active {
		if time.Now().After(deadline) {
			t.Fatal("code login did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return service.Status().Login
}

// A CLI that opened the browser itself receives the login through its local
// callback and exits without ever reading a pasted code.
func TestCodeLoginResolvesWhenCLIFinishesWithoutCode(t *testing.T) {
	script := writeScript(t, `
printf 'visit: https://code.test/authorize?x=1\n'
printf 'Paste code here if prompted > '
sleep 0.2
printf 'Login successful.\n'
`)
	completion := &recordedCodeCompletion{}
	service := testCodeService(script, completion)
	if _, err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if state := waitForCodeLogin(t, service); !state.Completed || state.Error != "" {
		t.Fatalf("login state = %#v", state)
	}
	calls, exitErr, output := completion.snapshot()
	if calls != 1 || exitErr != nil || !strings.HasSuffix(output, "Login successful.") {
		t.Fatalf("completion calls = %d, err = %v, output = %q", calls, exitErr, output)
	}
	// A code pasted after the CLI finished is not an error.
	if err := service.SubmitCode(context.Background(), "late-code"); err != nil {
		t.Fatalf("late code: %v", err)
	}
	if calls, _, _ := completion.snapshot(); calls != 1 {
		t.Fatalf("completion resolved %d times", calls)
	}
}

func TestCodeLoginPastedAfterCLIFailedReportsTheFailure(t *testing.T) {
	script := writeScript(t, `
printf 'visit: https://code.test/authorize?x=1\n'
sleep 0.2
printf 'OAuth error: state mismatch\n'
exit 1
`)
	completion := &recordedCodeCompletion{}
	service := testCodeService(script, completion)
	if _, err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForCodeLogin(t, service)
	err := service.SubmitCode(context.Background(), "pasted")
	if err != nil && strings.Contains(err.Error(), "stdin") {
		t.Fatalf("paste after exit reported a terminal write error: %v", err)
	}
	state := service.Status().Login
	if state.Completed || !strings.Contains(state.Error, "state mismatch") {
		t.Fatalf("login state = %#v", state)
	}
	if calls, _, _ := completion.snapshot(); calls != 1 {
		t.Fatalf("completion resolved %d times", calls)
	}
}

func TestCodeLoginCancelDoesNotResolveCompletion(t *testing.T) {
	script := writeScript(t, `
printf 'visit: https://code.test/authorize?x=1\n'
read code
`)
	completion := &recordedCodeCompletion{}
	service := testCodeService(script, completion)
	if _, err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := service.Cancel(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if calls, _, _ := completion.snapshot(); calls != 0 {
		t.Fatalf("cancelled login resolved %d times", calls)
	}
	if state := service.Status().Login; state != (CodeLoginState{}) {
		t.Fatalf("login state after cancel = %#v", state)
	}
}
