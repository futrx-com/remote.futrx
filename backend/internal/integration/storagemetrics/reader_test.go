package storagemetrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanBoundsAndSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(root, "file"), make([]byte, 8192), 0600)
	os.WriteFile(filepath.Join(outside, "private"), make([]byte, 1<<20), 0600)
	os.Symlink(outside, filepath.Join(root, "link"))
	result := scan(context.Background(), root, 100, 80)
	if result.Bytes == nil || *result.Bytes >= 1<<20 || result.Error != "" {
		t.Fatalf("%+v", result)
	}
	result = scan(context.Background(), root, 1, 80)
	if result.Bytes != nil || result.Error == "" {
		t.Fatal("partial scan reported complete")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result = scan(ctx, root, 100, 80); result.Bytes != nil {
		t.Fatal("cancelled sample returned total")
	}
}
func TestReaderRejectsUnmanagedPathAndSamplesAsynchronously(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project", "workspace")
	os.MkdirAll(cwd, 0700)
	reader := New(root, 80)
	if reader.Read(context.Background(), "/etc").Error == "" {
		t.Fatal("unmanaged path accepted")
	}
	if result := reader.Read(context.Background(), cwd); !result.Pending {
		t.Fatalf("first read %+v", result)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		result := reader.Read(context.Background(), cwd)
		if !result.Pending && result.Bytes != nil {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("sample did not complete")
}
