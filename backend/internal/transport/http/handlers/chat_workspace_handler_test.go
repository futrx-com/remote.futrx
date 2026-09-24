package httphandlers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	servicechat "github.com/futrx-com/remote.futrx.com/internal/service/chat"
	serviceworkspacefiles "github.com/futrx-com/remote.futrx.com/internal/service/workspacefiles"
	serviceworkspaceide "github.com/futrx-com/remote.futrx.com/internal/service/workspaceide"
)

type chatWorkspaceProjectResolver map[servicechat.ProjectID]string

func (r chatWorkspaceProjectResolver) WorkspaceForProject(
	_ context.Context,
	id servicechat.ProjectID,
) (string, error) {
	return r[id], nil
}

func TestHandleIDEOpenUsesTrustedProjectWorkspace(t *testing.T) {
	t.Parallel()
	const (
		projectsRoot = "/srv/workspace/projects"
		projectRoot  = projectsRoot + "/allowed/workspace"
	)
	chats := servicechat.New(
		nil,
		chatWorkspaceProjectResolver{"allowed": projectRoot},
		nil,
		nil,
		servicechat.WithHostWorkspaceRoot("/srv/workspace/remote.futrx"),
	)
	handler := &ChatHandler{
		chats: chats,
		ide:   serviceworkspaceide.New("https://code.remote.example/", projectsRoot),
	}
	request := httptest.NewRequest(http.MethodGet, "/?path=/workspace/README.md", nil)
	response := httptest.NewRecorder()

	handler.handleIDEOpen(response, request, servicechat.Meta{
		ID: "chat-1", ProjectID: "allowed", Cwd: "/srv/workspace/projects/other/workspace",
	})

	if response.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d; body=%q", response.Code, http.StatusFound, response.Body.String())
	}
	location, err := url.Parse(response.Header().Get("Location"))
	if err != nil {
		t.Fatalf("parse redirect: %v", err)
	}
	if location.Path != "/allowed/" {
		t.Fatalf("redirect path = %q, want allowed project", location.Path)
	}
	if folder := location.Query().Get("folder"); folder != "/workspace" {
		t.Fatalf("IDE folder = %q, want project container workspace", folder)
	}
}

type mediaRootStore struct {
	serviceworkspacefiles.Store
	root     string
	relative string
}

func (s *mediaRootStore) OpenFile(root, relative string) (io.ReadSeekCloser, string, time.Time, error) {
	s.root = root
	s.relative = relative
	return &mediaReadSeekCloser{Reader: bytes.NewReader([]byte("png"))}, "image.png", time.Time{}, nil
}

type mediaReadSeekCloser struct {
	*bytes.Reader
}

func (*mediaReadSeekCloser) Close() error { return nil }

func TestHandleMediaOpenUsesTrustedHostWorkspace(t *testing.T) {
	t.Parallel()
	const hostRoot = "/srv/workspace/remote.futrx"
	chats := servicechat.New(
		nil,
		nil,
		nil,
		nil,
		servicechat.WithHostWorkspaceRoot(hostRoot),
	)
	store := &mediaRootStore{}
	handler := &ChatHandler{
		chats: chats,
		files: serviceworkspacefiles.New(store),
	}
	request := httptest.NewRequest(http.MethodGet, "/?path=/workspace/image.png", nil)
	response := httptest.NewRecorder()

	handler.handleMediaOpen(response, request, servicechat.Meta{ID: "chat-1", Cwd: "/etc"})

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", response.Code, http.StatusOK, response.Body.String())
	}
	if store.root != hostRoot || store.relative != "image.png" {
		t.Fatalf("media store opened root=%q relative=%q, want %q and image.png", store.root, store.relative, hostRoot)
	}
}
