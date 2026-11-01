package files_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/port/files/outbound"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/repository/archives"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/service/files"
)

func TestArchiveBuildFailureReleasesResourcesForNextDownload(t *testing.T) {
	failure := errors.New("source read failed")
	source := &archiveSource{failure: failure}
	service := files.New("/workspace", source, archives.New(1, 64))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if file, err := service.DownloadArchive(ctx, "src"); file != nil || !errors.Is(err, failure) {
		t.Fatalf("failed download: file=%v err=%v", file, err)
	}
	source.failure = nil
	file, err := service.DownloadArchive(ctx, "src")
	if err != nil {
		t.Fatalf("next download failed: %v", err)
	}
	defer file.Close()
	content, err := io.ReadAll(file.Content())
	if err != nil || string(content) != "archive bytes" || file.Name != "src.zip" {
		t.Fatalf("download: name=%q content=%q error=%v", file.Name, content, err)
	}
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
