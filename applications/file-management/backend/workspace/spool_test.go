package workspace

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSpoolerCapsOutputAndRemovesFailedFile(t *testing.T) {
	directory := t.TempDir()
	spooler, err := NewSpooler(directory, t.TempDir(), 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := spooler.Prepare(context.Background(), func(destination io.Writer) error {
		_, writeErr := io.WriteString(destination, "12345")
		return writeErr
	})
	if archive != nil {
		_ = archive.Close()
		t.Fatal("oversized spool returned an archive")
	}
	if !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("prepare error = %v", err)
	}
	entries, readErr := os.ReadDir(directory)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed spool leaked %d entries", len(entries))
	}
	retry, err := spooler.Prepare(context.Background(), writePayload("1234"))
	if err != nil {
		t.Fatalf("failed spool did not release its shared slot: %v", err)
	}
	if err := retry.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSpoolerBoundsConcurrencyAndCleansOnClose(t *testing.T) {
	spooler, err := NewSpooler(t.TempDir(), t.TempDir(), 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	first, err := spooler.Prepare(context.Background(), writePayload("first"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Size() != int64(len("first")) {
		t.Fatalf("spooled size = %d, want %d", first.Size(), len("first"))
	}
	firstPath := first.file.Name()
	if _, err := os.Stat(firstPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open spool remains visible in the filesystem: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	second, err := spooler.Prepare(ctx, writePayload("second"))
	if second != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter = archive %v error %v", second, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(firstPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("spool remains after close: %v", err)
	}
	third, err := spooler.Prepare(context.Background(), writePayload("third"))
	if err != nil {
		t.Fatal(err)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestNewSpoolerRemovesCrashLeftoversOnlyWithinItsDirectory(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "spool")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "leftover.zip"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "keep")
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSpooler(directory, t.TempDir(), 1, 16); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, "leftover.zip")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("leftover still exists: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was touched: %v", err)
	}
}

func TestSpoolersShareArchiveSlotsAcrossApplicationInstances(t *testing.T) {
	sharedRuntime := t.TempDir()
	firstDirectory := t.TempDir()
	secondDirectory := t.TempDir()
	firstSpooler, err := NewSpooler(firstDirectory, sharedRuntime, 2, 16)
	if err != nil {
		t.Fatal(err)
	}
	secondSpooler, err := NewSpooler(secondDirectory, sharedRuntime, 2, 16)
	if err != nil {
		t.Fatal(err)
	}

	first, err := firstSpooler.Prepare(context.Background(), writePayload("first"))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := secondSpooler.Prepare(context.Background(), writePayload("second"))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	third, err := firstSpooler.Prepare(ctx, writePayload("third"))
	if third != nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("third shared archive = (%v, %v), want shared cap", third, err)
	}
	assertSpoolFileCount(t, firstDirectory, 0)
	assertSpoolFileCount(t, secondDirectory, 0)

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err = firstSpooler.Prepare(context.Background(), writePayload("third"))
	if err != nil {
		t.Fatalf("released shared slot was not reusable: %v", err)
	}
	if err := third.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSpoolerDoesNotArchiveItselfWhenDirectoryIsInsideWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	spooler, err := NewSpooler(
		filepath.Join(root, "data", "applications", "file-management", "spool"),
		t.TempDir(),
		1,
		1<<20,
	)
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	archive, err := spooler.Prepare(context.Background(), func(destination io.Writer) error {
		return store.WriteArchive(context.Background(), root, "", destination)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()

	reader, err := zip.NewReader(archive.file, archive.Size())
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 1 || reader.File[0].Name != "keep.txt" {
		t.Fatalf("archive entries = %+v, want only keep.txt", reader.File)
	}
}

func assertSpoolFileCount(t *testing.T, directory string, want int) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != want {
		t.Fatalf("spool entries in %q = %d, want %d", directory, len(entries), want)
	}
}

func writePayload(value string) func(io.Writer) error {
	return func(destination io.Writer) error {
		_, err := io.WriteString(destination, value)
		return err
	}
}
