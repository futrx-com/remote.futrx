package environment

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestApplyDiffEndsFlagParsingBeforeSecretValue(t *testing.T) {
	const secret = "-----secret-that-starts-like-a-flag"
	runner := &environmentTestRunner{}
	client := NewClient(runner)

	if err := client.ApplyDiff(context.Background(), "project", map[string]string{"SSH_KEY": secret}, nil); err != nil {
		t.Fatal(err)
	}

	want := [][]string{
		{"config", "get", "project", "environment.SSH_KEY"},
		{"config", "set", "project", "environment.SSH_KEY", "--", secret},
	}
	if !slices.EqualFunc(runner.calls, want, slices.Equal[[]string]) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestApplyDiffRemovesPersistentValueWhenReplacementIsMultiline(t *testing.T) {
	runner := &environmentTestRunner{}
	client := NewClient(runner)

	if err := client.ApplyDiff(context.Background(), "project", map[string]string{
		"SSH_KEY": "-----BEGIN OPENSSH PRIVATE KEY-----\nsecret\n-----END OPENSSH PRIVATE KEY-----",
	}, nil); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"config", "unset", "project", "environment.SSH_KEY"},
	}
	if !slices.EqualFunc(runner.calls, want, slices.Equal[[]string]) {
		t.Fatalf("calls = %#v, want %#v", runner.calls, want)
	}
}

func TestApplyDiffDoesNotIncludeSecretInSetError(t *testing.T) {
	const secret = "-----BEGIN-PRIVATE-KEY-secret"
	runner := &environmentTestRunner{setOut: secret, setErr: errors.New("exit status 1")}
	client := NewClient(runner)

	err := client.ApplyDiff(context.Background(), "project", map[string]string{"PRIVATE_KEY": secret}, nil)
	if err == nil {
		t.Fatal("expected set failure")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "BEGIN-PRIVATE-KEY") {
		t.Fatalf("error exposed secret: %v", err)
	}
	if !strings.Contains(err.Error(), "environment.PRIVATE_KEY") {
		t.Fatalf("error omitted safe key context: %v", err)
	}
}

type environmentTestRunner struct {
	calls    [][]string
	setOut   string
	setErr   error
	unsetOut string
	unsetErr error
}

func (r *environmentTestRunner) Available() bool { return true }

func (r *environmentTestRunner) Run(_ context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	if len(args) >= 2 && args[0] == "config" && args[1] == "set" {
		return r.setOut, r.setErr
	}
	if len(args) >= 2 && args[0] == "config" && args[1] == "unset" {
		return r.unsetOut, r.unsetErr
	}
	return "", nil
}

func (r *environmentTestRunner) RunStdin(ctx context.Context, _ io.Reader, args ...string) (string, error) {
	return r.Run(ctx, args...)
}

func TestApplyDiffIgnoresOnlyMissingEnvironmentKeys(t *testing.T) {
	for _, tc := range []struct {
		output  string
		wantErr bool
	}{
		{`Error: Cannot unset key "environment.SSH_KEY", it's not currently set`, false},
		{"Error: key is not set", false},
		{"Error: key doesn't exist", false},
		{"Error: permission denied", true},
	} {
		t.Run(tc.output, func(t *testing.T) {
			runner := &environmentTestRunner{unsetOut: tc.output, unsetErr: errors.New("exit status 1")}
			err := NewClient(runner).ApplyDiff(context.Background(), "project", nil, []string{"SSH_KEY", "OTHER_KEY"})
			if (err != nil) != tc.wantErr {
				t.Fatalf("ApplyDiff error = %v, want error = %v", err, tc.wantErr)
			}
			if !tc.wantErr && len(runner.calls) != 2 {
				t.Fatal("missing key prevented remaining environment cleanup")
			}
		})
	}
}
