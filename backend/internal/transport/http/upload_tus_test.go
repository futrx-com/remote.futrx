package httptransport

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	tusd "github.com/tus/tusd/v2/pkg/handler"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
)

type fixedUploadTarget struct {
	dir string
	err error
}

func (f fixedUploadTarget) UploadTarget(context.Context, servicechat.ID) (string, error) {
	return f.dir, f.err
}

// completeUpload stages a finished tus upload in tmpRoot and runs it through
// the completion consumer, as tusd would.
func completeUpload(t *testing.T, handler *UploadHandler, id string, meta map[string]string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(handler.tmpRoot, id), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(handler.tmpRoot, id+".info"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	events := make(chan tusd.HookEvent, 1)
	events <- tusd.HookEvent{Upload: tusd.FileInfo{ID: id, Size: 7, MetaData: meta}}
	close(events)
	handler.drainCompletions(events)
}

func assertMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("%s still exists (stat error %v)", path, err)
	}
}

func TestFinalizedUploadMovesIntoTheChatAndLeavesNothingBehind(t *testing.T) {
	tmpRoot, target := t.TempDir(), filepath.Join(t.TempDir(), ".uploads")
	handler := &UploadHandler{tmpRoot: tmpRoot, chats: fixedUploadTarget{dir: target}}

	completeUpload(t, handler, "abc123", map[string]string{"chatId": "chat-1", "filename": "notes.txt"})

	if data, err := os.ReadFile(filepath.Join(target, "notes.txt")); err != nil || string(data) != "payload" {
		t.Fatalf("moved upload = %q, %v; want payload", data, err)
	}
	assertMissing(t, filepath.Join(tmpRoot, "abc123"))
	assertMissing(t, filepath.Join(tmpRoot, "abc123.info"))
}

func TestFailedFinalizeDiscardsTheUploadsTempFiles(t *testing.T) {
	tests := []struct {
		name   string
		chats  fixedUploadTarget
		meta   map[string]string
		before func(t *testing.T, target string)
	}{
		{name: "missing chat id", meta: map[string]string{"filename": "notes.txt"}},
		{name: "invalid filename", meta: map[string]string{"chatId": "chat-1", "filename": ".."}},
		{name: "chat cannot be resolved", chats: fixedUploadTarget{err: errors.New("chat not found")}, meta: map[string]string{"chatId": "chat-1", "filename": "notes.txt"}},
		{
			name: "destination exists",
			meta: map[string]string{"chatId": "chat-1", "filename": "notes.txt"},
			before: func(t *testing.T, target string) {
				if err := os.MkdirAll(target, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(target, "notes.txt"), []byte("existing"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tmpRoot, target := t.TempDir(), filepath.Join(t.TempDir(), ".uploads")
			chats := test.chats
			if chats.err == nil {
				chats.dir = target
			}
			if test.before != nil {
				test.before(t, target)
			}
			handler := &UploadHandler{tmpRoot: tmpRoot, chats: chats}

			completeUpload(t, handler, "abc123", test.meta)

			assertMissing(t, filepath.Join(tmpRoot, "abc123"))
			assertMissing(t, filepath.Join(tmpRoot, "abc123.info"))
			if test.before != nil {
				if data, _ := os.ReadFile(filepath.Join(target, "notes.txt")); string(data) != "existing" {
					t.Fatalf("existing file = %q, want it untouched", data)
				}
			}
		})
	}
}

func TestDiscardIgnoresIDsThatAreNotPlainNames(t *testing.T) {
	tmpRoot := t.TempDir()
	outside := filepath.Join(filepath.Dir(tmpRoot), "keep-me")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	handler := &UploadHandler{tmpRoot: tmpRoot}

	handler.discard("../keep-me")
	handler.discard("")

	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("discard reached outside tmpRoot: %v", err)
	}
}
