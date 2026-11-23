package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	appLifecycle "futrx.local/catalog/applications/s3disk/backend/lifecycle"
	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// pushBackend is a backend whose container commands are recorded rather than
// run, with `mountpoint` and `mkdir` succeeding so a test only has to say what
// the interesting command does.
func pushBackend(t *testing.T, respond func(args []string) commandResult) (*backend, *[][]string) {
	t.Helper()
	b := testBackend(t)
	var calls [][]string
	b.run = func(_ context.Context, _ string, args ...string) commandResult {
		calls = append(calls, args)
		switch args[0] {
		case "mountpoint", "mkdir", "rm":
			return commandResult{}
		}
		return respond(args)
	}
	return b, &calls
}

func pushNames(t *testing.T, b *backend, names ...string) (applications.Response, map[string]any) {
	t.Helper()
	body, err := json.Marshal(pushRequest{Names: names})
	if err != nil {
		t.Fatal(err)
	}
	response, _ := b.Handle(applications.Request{Method: "POST", Path: "push", Body: body})
	decoded := map[string]any{}
	if err := json.Unmarshal(response.Body, &decoded); err != nil {
		t.Fatalf("body %s: %v", response.Body, err)
	}
	return response, decoded
}

func TestPushMovesUploadsIntoTheMountInsideTheContainer(t *testing.T) {
	b, calls := pushBackend(t, func(args []string) commandResult {
		if args[0] == "test" {
			return commandResult{Error: "exit status 1"} // not there yet
		}
		return commandResult{}
	})
	events := &recordingPushEvents{}
	b.pushes = events
	response, decoded := pushNames(t, b, "shot-a8ho.png")
	if response.Status != http.StatusOK {
		t.Fatalf("status %d: %s", response.Status, response.Body)
	}
	if decoded["stored"] != float64(1) || decoded["removed"] != float64(1) {
		t.Fatalf("stored %v removed %v", decoded["stored"], decoded["removed"])
	}
	if decoded["directory"] != "/workspace/s3/uploads" {
		t.Fatalf("directory %v", decoded["directory"])
	}
	if want := (appLifecycle.PushOutcome{Directory: "/workspace/s3/uploads", Requested: 1, Stored: 1, Removed: 1}); len(events.outcomes) != 1 || events.outcomes[0] != want {
		t.Fatalf("push events = %+v, want %+v", events.outcomes, want)
	}
	// Both commands have to be the container's own, so the write goes through
	// the FUSE mount rather than the host directory underneath it — and the
	// delete has to come after the copy, never before.
	copyCall := []string{"cp", "--", "/workspace/.uploads/shot-a8ho.png", "/workspace/s3/uploads/shot-a8ho.png"}
	deleteCall := []string{"rm", "-f", "--", "/workspace/.uploads/shot-a8ho.png"}
	copied, deleted := -1, -1
	for i, call := range *calls {
		if reflect.DeepEqual(call, copyCall) {
			copied = i
		}
		if reflect.DeepEqual(call, deleteCall) {
			deleted = i
		}
	}
	if copied < 0 || deleted < 0 || deleted < copied {
		t.Fatalf("copy at %d, delete at %d in %v", copied, deleted, *calls)
	}
}

func TestPushKeepsTheUploadWhenTheCopyFails(t *testing.T) {
	// Deleting here would destroy the only copy of the attachment.
	b, calls := pushBackend(t, func(args []string) commandResult {
		if args[0] == "test" {
			return commandResult{Error: "exit status 1"}
		}
		return commandResult{Output: "cp: no space left on device", Error: "exit status 1"}
	})
	events := &recordingPushEvents{}
	b.pushes = events
	_, decoded := pushNames(t, b, "shot.png")
	if decoded["stored"] != float64(0) || decoded["removed"] != float64(0) {
		t.Fatalf("stored %v removed %v", decoded["stored"], decoded["removed"])
	}
	if want := (appLifecycle.PushOutcome{Directory: "/workspace/s3/uploads", Requested: 1, Issues: 1}); len(events.outcomes) != 1 || events.outcomes[0] != want {
		t.Fatalf("push events = %+v, want %+v", events.outcomes, want)
	}
	for _, call := range *calls {
		if call[0] == "rm" {
			t.Fatalf("deleted after a failed copy: %v", call)
		}
	}
}

func TestPushKeepsTheUploadWhenWritebackIsAsynchronous(t *testing.T) {
	// With --async-writeback a closed file may still be only in the local
	// cache, so .uploads is not a redundant copy yet.
	b := newBackend(appLifecycle.NewOperations(), &recordingPushEvents{})
	instance := testInstance()
	instance.Env["S3DISK_MOUNT_ARGS"] = "--exclusive --async-writeback"
	if err := b.Init(instance); err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	b.run = func(_ context.Context, _ string, args ...string) commandResult {
		calls = append(calls, args)
		if args[0] == "test" {
			return commandResult{Error: "exit status 1"}
		}
		return commandResult{}
	}
	_, decoded := pushNames(t, b, "shot.png")
	if decoded["stored"] != float64(1) || decoded["removed"] != float64(0) {
		t.Fatalf("stored %v removed %v", decoded["stored"], decoded["removed"])
	}
	for _, call := range calls {
		if call[0] == "rm" {
			t.Fatalf("deleted while write-back is asynchronous: %v", call)
		}
	}
}

func TestPushRefusesNamesThatAreNotPlainFileNames(t *testing.T) {
	for _, name := range []string{
		"", ".", "..", "../../etc/shadow", "sub/dir.png", "back\\slash.png", "-rf.png",
		strings.Repeat("a", 256),
	} {
		b, calls := pushBackend(t, func([]string) commandResult { return commandResult{} })
		_, decoded := pushNames(t, b, name)
		if decoded["stored"] != float64(0) {
			t.Fatalf("%q was accepted", name)
		}
		for _, call := range *calls {
			if call[0] == "cp" {
				t.Fatalf("%q reached cp as %v", name, call)
			}
		}
	}
}

func TestPushRefusesToWriteUnderAnUnmountedMountpoint(t *testing.T) {
	// Writing here would land in the plain directory the mount hides, which
	// looks like success and never reaches the bucket.
	b := testBackend(t)
	events := &recordingPushEvents{}
	b.pushes = events
	var copied bool
	b.run = func(_ context.Context, _ string, args ...string) commandResult {
		if args[0] == "cp" {
			copied = true
		}
		if args[0] == "mountpoint" {
			return commandResult{Error: "exit status 32"}
		}
		return commandResult{}
	}
	response, _ := pushNames(t, b, "shot.png")
	if response.Status != http.StatusConflict {
		t.Fatalf("status %d: %s", response.Status, response.Body)
	}
	if copied {
		t.Fatal("copied into an unmounted mountpoint")
	}
	if len(events.outcomes) != 0 {
		t.Fatalf("published a push that did not run: %+v", events.outcomes)
	}
}

func TestPushPublicationFailureKeepsTheCopyResult(t *testing.T) {
	b, _ := pushBackend(t, func(args []string) commandResult {
		if args[0] == "test" {
			return commandResult{Error: "not found"}
		}
		return commandResult{}
	})
	events := &recordingPushEvents{err: errors.New("bus unavailable")}
	b.pushes = events
	response, decoded := pushNames(t, b, "shot.png")
	if response.Status != http.StatusOK || decoded["stored"] != float64(1) || decoded["removed"] != float64(1) {
		t.Fatalf("copy result changed: status %d body %s", response.Status, response.Body)
	}
	if warning, _ := decoded["warning"].(string); !strings.Contains(warning, "event not published: bus unavailable") {
		t.Fatalf("warning = %q", warning)
	}
	if len(events.outcomes) != 1 {
		t.Fatalf("publication attempts = %d, want 1", len(events.outcomes))
	}
}

func TestPushReportsDirectoryCreationFailureBeforeCopying(t *testing.T) {
	b := testBackend(t)
	var copied bool
	b.run = func(_ context.Context, _ string, args ...string) commandResult {
		if args[0] == "mkdir" {
			return commandResult{Output: "mkdir: permission denied", Error: "exit status 1"}
		}
		if args[0] == "cp" {
			copied = true
		}
		return commandResult{}
	}
	response, _ := pushNames(t, b, "shot.png")
	if response.Status != http.StatusBadGateway ||
		!strings.Contains(string(response.Body), "Could not create /workspace/s3/uploads: mkdir: permission denied") {
		t.Fatalf("directory error changed: status %d body %s", response.Status, response.Body)
	}
	if copied {
		t.Fatal("copied after mkdir failed")
	}
}

func TestPushReportsAnAlreadyStoredFileWithoutCopyingItAgain(t *testing.T) {
	b, calls := pushBackend(t, func(args []string) commandResult {
		return commandResult{} // `test -e` succeeds: it is already there
	})
	_, decoded := pushNames(t, b, "shot.png")
	// Still removed: the bucket has it, so .uploads is the redundant copy.
	if decoded["stored"] != float64(1) || decoded["removed"] != float64(1) {
		t.Fatalf("stored %v removed %v", decoded["stored"], decoded["removed"])
	}
	results, _ := decoded["results"].([]any)
	first, _ := results[0].(map[string]any)
	if first["skipped"] != true {
		t.Fatalf("expected a skip, got %v", first)
	}
	for _, call := range *calls {
		if call[0] == "cp" {
			t.Fatalf("re-copied an existing object: %v", call)
		}
	}
}

func TestPushBoundsTheBatchAndRedactsFailures(t *testing.T) {
	b, _ := pushBackend(t, func(args []string) commandResult {
		if args[0] == "test" {
			return commandResult{Error: "exit status 1"}
		}
		return commandResult{Output: "cp: cannot stat private-value", Error: "exit status 1"}
	})
	response, decoded := pushNames(t, b, "shot.png")
	if response.Status != http.StatusOK || decoded["stored"] != float64(0) {
		t.Fatalf("status %d body %s", response.Status, response.Body)
	}
	if strings.Contains(string(response.Body), "private-value") {
		t.Fatalf("credential leaked: %s", response.Body)
	}

	names := make([]string, maxPushNames+1)
	for i := range names {
		names[i] = "shot.png"
	}
	response, _ = pushNames(t, b, names...)
	if response.Status != http.StatusBadRequest {
		t.Fatalf("oversized batch accepted: %d", response.Status)
	}
}
