package workspace

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSpoolerCapsOutputAndRemovesFailedFile(t *testing.T) {
	directory := t.TempDir()
	spooler, err := NewSpooler(directory, 1, 4)
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
	if len(entries) != 0 || len(spooler.slots) != 0 {
		t.Fatalf("failed spool leaked entries=%d slots=%d", len(entries), len(spooler.slots))
	}
}

func TestSpoolerBoundsConcurrencyAndCleansOnClose(t *testing.T) {
	spooler, err := NewSpooler(t.TempDir(), 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	first, err := spooler.Prepare(context.Background(), writePayload("first"))
	if err != nil {
		t.Fatal(err)
	}
	firstPath := first.file.Name()
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
	if _, err := NewSpooler(directory, 1, 16); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, "leftover.zip")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("leftover still exists: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was touched: %v", err)
	}
}

func writePayload(value string) func(io.Writer) error {
	return func(destination io.Writer) error {
		_, err := io.WriteString(destination, value)
		return err
	}
}
