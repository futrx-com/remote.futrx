package workspace

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestServiceUsesTrustedRootAndNormalizesRelativePaths(t *testing.T) {
	root, _ := setupWorkspace(t)
	service := NewService(NewStore())
	listing, err := service.List(root, "/src/../src")
	if err != nil {
		t.Fatal(err)
	}
	if listing.Path != "src" || len(listing.Entries) != 1 || listing.Entries[0].Name != "app.go" {
		t.Fatalf("listing = %+v", listing)
	}
	if _, err := service.List("relative", ""); !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("relative root error = %v", err)
	}
}

func TestServiceSearchRequiresTwoCharacters(t *testing.T) {
	root, _ := setupWorkspace(t)
	result, err := NewService(NewStore()).Search(root, "a")
	if err != nil {
		t.Fatal(err)
	}
	if result.Entries == nil || len(result.Entries) != 0 {
		t.Fatalf("short search result = %+v", result)
	}
}

func TestServiceOpensSupportedMediaOnly(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "image.PNG"), []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "code.go"), []byte("go"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService(NewStore())
	media, err := service.OpenMedia(root, "image.PNG")
	if err != nil {
		t.Fatal(err)
	}
	content, readErr := io.ReadAll(media.Content())
	if err := errors.Join(readErr, media.Close()); err != nil {
		t.Fatal(err)
	}
	if media.ContentType != "image/png" || string(content) != "png" {
		t.Fatalf("media = type %q body %q", media.ContentType, content)
	}
	if _, err := service.OpenMedia(root, "code.go"); !errors.Is(err, ErrUnsupportedMedia) {
		t.Fatalf("unsupported media error = %v", err)
	}
}

func TestPrepareArchiveNamesRootAndFolder(t *testing.T) {
	root, _ := setupWorkspace(t)
	service := NewService(NewStore())
	rootArchive, err := service.PrepareArchive(root, "")
	if err != nil {
		t.Fatal(err)
	}
	folderArchive, err := service.PrepareArchive(root, "src")
	if err != nil {
		t.Fatal(err)
	}
	if rootArchive.Name != "workspace.zip" || folderArchive.Name != "src.zip" {
		t.Fatalf("archive names = %q, %q", rootArchive.Name, folderArchive.Name)
	}
}
