package main

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
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

func pushNames(t *testing.T, b *backend, names ...string) (appplugin.Response, map[string]any) {
	t.Helper()
	body, err := json.Marshal(pushRequest{Names: names})
	if err != nil {
		t.Fatal(err)
	}
	response, _ := b.Handle(appplugin.Request{Method: "POST", Path: "push", Body: body})
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
	_, decoded := pushNames(t, b, "shot.png")
	if decoded["stored"] != float64(0) || decoded["removed"] != float64(0) {
		t.Fatalf("stored %v removed %v", decoded["stored"], decoded["removed"])
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
	b := newBackend()
	if err := b.Init(appplugin.Instance{
		Scope:         "project",
		ContainerName: "project-test",
		Env:           map[string]string{"S3DISK_MOUNT_ARGS": "--exclusive --async-writeback"},
	}); err != nil {
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

func TestUploadsDirIsAPlainRelativePath(t *testing.T) {
	for input, want := range map[string]string{"": "uploads", ".": "", "/attachments/": "attachments", "a/b": "a/b"} {
		got, err := uploadsDirOf(input)
		if err != nil || got != want {
			t.Fatalf("uploadsDirOf(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"..", "a/../..", "-flag", "a/-flag", "a//b"} {
		if _, err := uploadsDirOf(input); err == nil {
			t.Fatalf("uploadsDirOf(%q) was accepted", input)
		}
	}
}
