package workspace

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
	serviceprofiles "github.com/futrx-com/remote.futrx.com/internal/service/container/profiles"
	"github.com/futrx-com/remote.futrx.com/internal/service/harness/provisioning"
)

func TestSystemInstructionsPublishNativeFormatsAndRepairChangedFiles(t *testing.T) {
	dir := t.TempDir()
	plain := provisioning.InstructionTarget{Path: filepath.Join(dir, "plain", "system.md"), HashPath: filepath.Join(dir, "plain", ".hash")}
	wrapped := provisioning.InstructionTarget{Path: filepath.Join(dir, "wrapped", "SYSTEM.md"), HashPath: filepath.Join(dir, "wrapped", ".hash"), Prefix: "${base_prompt}\n\n"}
	runner := &instructionFilesRunner{}
	p := NewProvisioner(runner, serviceprofiles.NewCatalog([]provisioning.Profile{
		{Instructions: &plain}, {Instructions: &wrapped},
	}), assets.NewPublisher(runner), []byte("trusted platform instructions"))
	ensure := func() {
		t.Helper()
		text, err := p.EnsureHarnessInstructions(context.Background(), "container")
		if err != nil || text != "trusted platform instructions" {
			t.Fatalf("instructions = %q, error = %v", text, err)
		}
		for _, target := range []provisioning.InstructionTarget{plain, wrapped} {
			content, err := os.ReadFile(target.Path)
			if err != nil || string(content) != string(target.Render([]byte(text))) {
				t.Fatalf("%s = %q, error = %v", target.Path, content, err)
			}
		}
	}
	ensure()
	ensure()
	if runner.pushes != 2 {
		t.Fatalf("unchanged files were rewritten: %d pushes", runner.pushes)
	}
	if err := os.WriteFile(wrapped.Path, []byte("changed from inside the container"), 0644); err != nil {
		t.Fatal(err)
	}
	ensure()
	if err := os.Remove(plain.Path); err != nil {
		t.Fatal(err)
	}
	ensure()
	if runner.pushes != 4 {
		t.Fatalf("changed and missing files were not repaired: %d pushes", runner.pushes)
	}
}

func TestSystemInstructionsMigrateOnlyUneditedManagedFiles(t *testing.T) {
	for _, test := range []struct {
		name, legacy              string
		marker, failPush, removed bool
	}{
		{name: "old managed template", legacy: "previous release", marker: true, removed: true},
		{name: "user edit", legacy: "user instructions", marker: true},
		{name: "unmanaged file", legacy: "previous release"},
		{name: "failed publication", legacy: "previous release", marker: true, failPush: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			target := provisioning.InstructionTarget{
				Path: filepath.Join(dir, "system.md"), HashPath: filepath.Join(dir, ".new-hash"),
				LegacyPath: filepath.Join(dir, "AGENTS.md"), LegacyHashPath: filepath.Join(dir, ".old-hash"),
			}
			if err := os.WriteFile(target.LegacyPath, []byte(test.legacy), 0644); err != nil {
				t.Fatal(err)
			}
			if test.marker {
				if err := os.WriteFile(target.LegacyHashPath, []byte(assets.Hash([]byte("previous release"))), 0644); err != nil {
					t.Fatal(err)
				}
			}
			runner := &instructionFilesRunner{failPush: test.failPush}
			p := NewProvisioner(runner, serviceprofiles.NewCatalog([]provisioning.Profile{{Instructions: &target}}), assets.NewPublisher(runner), []byte("new platform instructions"))
			text, err := p.EnsureHarnessInstructions(context.Background(), "container")
			if (err != nil) != test.failPush {
				t.Fatalf("publication error = %v", err)
			}
			if err != nil && text != "" {
				t.Fatalf("failed preparation returned instructions: %q", text)
			}
			_, err = os.Stat(target.LegacyPath)
			if errors.Is(err, os.ErrNotExist) != test.removed {
				t.Fatalf("legacy file stat = %v, want removed %t", err, test.removed)
			}
			if test.marker {
				if _, err := os.Stat(target.LegacyHashPath); err != nil {
					t.Fatalf("shared legacy marker was removed: %v", err)
				}
			}
		})
	}
}

// Exercise the actual publication and migration commands using isolated files,
// without LXD or provider credentials.
type instructionFilesRunner struct {
	pushes   int
	failPush bool
}

func (*instructionFilesRunner) Available() bool { return true }
func (r *instructionFilesRunner) Run(ctx context.Context, args ...string) (string, error) {
	if args[0] == "exec" {
		out, err := exec.CommandContext(ctx, args[3], args[4:]...).CombinedOutput()
		return string(out), err
	}
	if args[1] == "push" {
		if r.failPush {
			return "", errors.New("push failed")
		}
		content, err := os.ReadFile(args[3])
		if err != nil {
			return "", err
		}
		r.pushes++
		return "", os.WriteFile(args[4][len("container"):], content, 0644)
	}
	content, err := os.ReadFile(args[2][len("container"):])
	return string(content), err
}
func (*instructionFilesRunner) RunStdin(ctx context.Context, input io.Reader, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, args[3], args[4:]...)
	cmd.Stdin = input
	out, err := cmd.CombinedOutput()
	return string(out), err
}
