package files_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"testing"
	"time"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/port/files/outbound"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/repository/archives"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/service/files"
)

func TestArchiveBuildFailurePublishesNothing(t *testing.T) {
	root := t.TempDir()
	failure := errors.New("source read failed")
	source := &archiveSource{failure: failure}
	service := files.New(root, source, archives.New(root, 64, time.Hour))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if published, err := service.PublishArchive(ctx, "src"); published.Path != "" || !errors.Is(err, failure) {
		t.Fatalf("failed archive: published=%+v err=%v", published, err)
	}
	if folders := archiveFolders(t, root); len(folders) != 0 {
		t.Fatalf("failed archive left %v", folders)
	}

	source.failure = nil
	published, err := service.PublishArchive(ctx, "src")
	if err != nil {
		t.Fatalf("next archive failed: %v", err)
	}
	if path.Dir(path.Dir(published.Path)) != config.DownloadsDirectory || path.Base(published.Path) != "src.zip" {
		t.Fatalf("published at %q", published.Path)
	}
	content, err := os.ReadFile(filepath.Join(root, published.Path))
	if err != nil || string(content) != "archive bytes" {
		t.Fatalf("published content = %q, %v", content, err)
	}
}

func archiveFolders(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, config.DownloadsDirectory))
	if err != nil {
		t.Fatal(err)
	}
	var folders []string
	for _, entry := range entries {
		if entry.IsDir() {
			folders = append(folders, entry.Name())
		}
	}
	return folders
}

type archiveSource struct {
	outbound.Store
	failure error
}

func (*archiveSource) DirectoryExists(string, string) bool { return true }

func (s *archiveSource) WriteArchive(_ context.Context, _, _ string, destination io.Writer) error {
	if s.failure != nil {
		return s.failure
	}
	_, err := io.WriteString(destination, "archive bytes")
	return err
}
