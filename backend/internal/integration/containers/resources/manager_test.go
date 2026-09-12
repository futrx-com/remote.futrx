package resources

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
)

type fakeRunner struct {
	responses map[string]fakeResponse
	calls     []string
}

type fakeResponse struct {
	out string
	err error
}

func (f *fakeRunner) Available() bool { return true }

func (f *fakeRunner) Run(_ context.Context, args ...string) (string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	r := f.responses[key]
	return r.out, r.err
}

func (f *fakeRunner) RunStdin(ctx context.Context, _ io.Reader, args ...string) (string, error) {
	return f.Run(ctx, args...)
}

func (f *fakeRunner) called(prefix string) []string {
	var out []string
	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

func TestEnsureCreatesProfileAndAttaches(t *testing.T) {
	runner := &fakeRunner{responses: map[string]fakeResponse{
		"profile show " + ProfileName: {err: errors.New("not found")},
		"config show c1":              {out: "profiles:\n- default\n"},
	}}

	if err := NewManager(runner).Ensure(context.Background(), "c1"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if got := runner.called("profile create"); len(got) != 1 {
		t.Fatalf("expected one profile create, got %v", got)
	}
	if want := len(NewManager(runner).profileEntries()); len(runner.called("profile set")) != want {
		t.Fatalf("expected %d profile set calls, got %v", want, runner.called("profile set"))
	}
	if got := runner.called("profile add c1"); len(got) != 1 {
		t.Fatalf("expected profile add, got %v", got)
	}
}

func TestEnsureConvergedIsReadOnly(t *testing.T) {
	responses := map[string]fakeResponse{
		"profile show " + ProfileName: {out: "name: " + ProfileName},
		"config show c1":              {out: "profiles:\n- default\n- " + ProfileName + "\n"},
	}
	for _, kv := range NewManager(&fakeRunner{}).profileEntries() {
		responses["profile get "+ProfileName+" "+kv[0]] = fakeResponse{out: kv[1] + "\n"}
	}
	runner := &fakeRunner{responses: responses}

	if err := NewManager(runner).Ensure(context.Background(), "c1"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	for _, mutating := range []string{"profile create", "profile set", "profile add"} {
		if got := runner.called(mutating); len(got) != 0 {
			t.Fatalf("converged state must not mutate; got %v", got)
		}
	}
}

func TestEnsureRepairsDriftedKey(t *testing.T) {
	responses := map[string]fakeResponse{
		"profile show " + ProfileName: {out: "name: " + ProfileName},
		"config show c1":              {out: "profiles:\n- default\n- " + ProfileName + "\n"},
	}
	for _, kv := range NewManager(&fakeRunner{}).profileEntries() {
		responses["profile get "+ProfileName+" "+kv[0]] = fakeResponse{out: kv[1] + "\n"}
	}
	// One key drifted (hand-edited profile).
	responses["profile get "+ProfileName+" limits.memory"] = fakeResponse{out: "16GiB\n"}
	runner := &fakeRunner{responses: responses}

	if err := NewManager(runner).Ensure(context.Background(), "c1"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	got := runner.called("profile set")
	if len(got) != 1 || !strings.Contains(got[0], "limits.memory 4GiB") {
		t.Fatalf("expected exactly the drifted key to be reset, got %v", got)
	}
}

func TestSetLimitsAppliesContainerOverrides(t *testing.T) {
	runner := &fakeRunner{responses: map[string]fakeResponse{
		"profile device get default root pool": {out: "tank\n"},
		"storage show tank":                    {out: "name: tank\ndriver: zfs\n"},
	}}

	if err := NewManager(runner).SetLimits(context.Background(), "c1", "4", "8GiB", "40GiB"); err != nil {
		t.Fatalf("SetLimits: %v", err)
	}

	want := []string{
		"config set c1 limits.cpu 4",
		"config set c1 limits.memory 8GiB",
		"profile device get default root pool",
		"storage show tank",
		"config device override c1 root size=40GiB",
	}
	if !slices.Equal(runner.calls, want) {
		t.Fatalf("calls:\n got: %q\nwant: %q", runner.calls, want)
	}
}

func TestSetLimitsClearsContainerOverrides(t *testing.T) {
	runner := &fakeRunner{responses: map[string]fakeResponse{}}

	if err := NewManager(runner).SetLimits(context.Background(), "c1", "", "", ""); err != nil {
		t.Fatalf("SetLimits: %v", err)
	}

	want := []string{
		"config unset c1 limits.cpu",
		"config unset c1 limits.memory",
		"config device unset c1 root size",
	}
	if !slices.Equal(runner.calls, want) {
		t.Fatalf("calls:\n got: %q\nwant: %q", runner.calls, want)
	}
}
