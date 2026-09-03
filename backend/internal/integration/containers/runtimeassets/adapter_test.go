package runtimeassets

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/internal/agent/provisioning"
	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/assets"
)

func TestEnsurePublishesSelectedProfileAssets(t *testing.T) {
	runner := &runtimeAssetRunner{available: true}
	adapter := NewAdapter(runner, assets.NewPublisher(runner))
	err := adapter.Ensure(context.Background(), "project-container", []provisioning.RuntimeAsset{{
		Content:       []byte("runtime-config"),
		Path:          "/root/.provider/catalog.json",
		HashPath:      "/root/.provider/.catalog.sha256",
		Mode:          "640",
		Directory:     "/root/.provider",
		DirectoryMode: "700",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(runner.calls, func(call string) bool {
		return strings.Contains(call, "install -d -m 700 /root/.provider")
	}) {
		t.Fatalf("mkdir calls = %#v", runner.calls)
	}
	if !slices.ContainsFunc(runner.calls, func(call string) bool {
		return strings.Contains(call, "file push --mode=640") &&
			strings.Contains(call, "project-container/root/.provider/catalog.json")
	}) {
		t.Fatalf("push calls = %#v", runner.calls)
	}
	if string(runner.pushed) != "runtime-config" {
		t.Fatalf("pushed content = %q", runner.pushed)
	}
	if runner.hashInput != assets.Hash([]byte("runtime-config")) {
		t.Fatalf("hash marker = %q", runner.hashInput)
	}
}

func TestEnsureSkipsRunnerForEmptyProfile(t *testing.T) {
	runner := &runtimeAssetRunner{available: false}
	adapter := NewAdapter(runner, assets.NewPublisher(runner))
	if err := adapter.Ensure(context.Background(), "project-container", nil); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("calls = %#v", runner.calls)
	}
}

func TestEnsureRepublishesTamperedContentDespiteCurrentMarker(t *testing.T) {
	content := []byte("trusted-runtime-config")
	runner := &runtimeAssetRunner{
		available:     true,
		markerContent: assets.Hash(content),
		pulledContent: "tampered-runtime-config",
	}
	adapter := NewAdapter(runner, assets.NewPublisher(runner))
	err := adapter.Ensure(context.Background(), "project-container", []provisioning.RuntimeAsset{{
		Content:   content,
		Path:      "/root/.provider/catalog.json",
		HashPath:  "/root/.provider/.catalog.sha256",
		Directory: "/root/.provider",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if string(runner.pushed) != string(content) {
		t.Fatalf("pushed content = %q, want verified template", runner.pushed)
	}
	if !slices.ContainsFunc(runner.calls, func(call string) bool {
		return strings.Contains(call, "install -d -m 700 /root/.provider")
	}) {
		t.Fatalf("default mkdir calls = %#v", runner.calls)
	}
	if !slices.ContainsFunc(runner.calls, func(call string) bool {
		return strings.Contains(call, "file push --mode=644")
	}) {
		t.Fatalf("default push calls = %#v", runner.calls)
	}
}

type runtimeAssetRunner struct {
	available     bool
	calls         []string
	pushed        []byte
	hashInput     string
	markerContent string
	pulledContent string
}

func (r *runtimeAssetRunner) Available() bool {
	return r.available
}

func (r *runtimeAssetRunner) Run(_ context.Context, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	if len(args) >= 4 && args[0] == "exec" && args[3] == "cat" {
		if r.markerContent != "" {
			return r.markerContent, nil
		}
		return "", errors.New("missing marker")
	}
	if len(args) >= 4 && args[0] == "file" && args[1] == "pull" {
		return r.pulledContent, nil
	}
	if len(args) >= 5 && args[0] == "file" && args[1] == "push" {
		content, err := os.ReadFile(args[3])
		if err != nil {
			return "", err
		}
		r.pushed = content
	}
	return "", nil
}

func (r *runtimeAssetRunner) RunStdin(_ context.Context, stdin io.Reader, args ...string) (string, error) {
	r.calls = append(r.calls, strings.Join(args, " "))
	content, err := io.ReadAll(stdin)
	if err != nil {
		return "", err
	}
	r.hashInput = string(content)
	return "", nil
}
