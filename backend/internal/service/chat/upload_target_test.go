package chat

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

type uploadRepository struct {
	Repository
	meta Meta
}

func (r *uploadRepository) Get(context.Context, ID) (Meta, error) { return r.meta, nil }

type uploadProjects struct {
	workspace string
	err       error
}

func (p uploadProjects) WorkspaceForProject(context.Context, ProjectID) (string, error) {
	return p.workspace, p.err
}

func uploadTarget(t *testing.T, projects ProjectResolver) (string, error) {
	t.Helper()
	repo := &uploadRepository{meta: Meta{ID: "deadbeef", ProjectID: "p1", Cwd: "/chat/cwd"}}
	return New(repo, projects, nil, nil).UploadTarget(context.Background(), "deadbeef")
}

// The workspace root is what the frontend predicts the upload path from, so it
// wins over the chat's own directory whenever it can be resolved.
func TestUploadTargetPrefersTheProjectWorkspace(t *testing.T) {
	got, err := uploadTarget(t, uploadProjects{workspace: "/workspace/p1"})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/workspace/p1", ".uploads"); got != want {
		t.Errorf("target = %q, want %q", got, want)
	}
}

// A chat outlives the project it belonged to. Refusing its attachments because
// that project is gone would break a conversation the user can still open and
// still read, so it falls back to the directory it was working in.
func TestUploadTargetFallsBackWhenTheProjectIsGone(t *testing.T) {
	got, err := uploadTarget(t, uploadProjects{err: ErrProjectNotFound})
	if err != nil {
		t.Fatalf("a deleted project failed the lookup: %v", err)
	}
	if want := filepath.Join("/chat/cwd", ".uploads"); got != want {
		t.Errorf("target = %q, want %q", got, want)
	}
}

// Any other failure means the workspace exists but could not be resolved.
// Writing into the chat's cwd instead would put the file somewhere neither the
// user nor the frontend is looking for it.
func TestUploadTargetFailsWhenTheWorkspaceCannotBeResolved(t *testing.T) {
	unavailable := errors.New("project store unavailable")
	if _, err := uploadTarget(t, uploadProjects{err: unavailable}); !errors.Is(err, unavailable) {
		t.Errorf("err = %v, want %v", err, unavailable)
	}
}
