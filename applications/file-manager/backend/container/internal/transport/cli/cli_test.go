package cli_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"futrx.local/catalog/applications/file-manager/backend/container/internal/config"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/repository/archives"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/repository/workspace"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/service/files"
	"futrx.local/catalog/applications/file-manager/backend/container/internal/transport/cli"
)

type result struct {
	status int
	stdout []byte
	stderr string
}

func run(t *testing.T, service *files.Service, command, request string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	status := cli.Run(context.Background(), []string{command}, strings.NewReader(request), &stdout, &stderr, service)
	return result{status: status, stdout: stdout.Bytes(), stderr: stderr.String()}
}

func TestCommands(t *testing.T) {
	root := setupWorkspace(t)
	service := newService(root, config.MaxArchiveBytes)

	t.Run("listing", func(t *testing.T) {
		got := run(t, service, "list", `{}`)
		var listing struct {
			Path    string `json:"path"`
			Entries []struct {
				Name  string `json:"name"`
				IsDir bool   `json:"isDir"`
			} `json:"entries"`
			Truncated bool `json:"truncated"`
		}
		if got.status != cli.ExitOK || json.Unmarshal(got.stdout, &listing) != nil {
			t.Fatalf("list: %d %s %s", got.status, got.stdout, got.stderr)
		}
		if len(listing.Entries) != 2 || listing.Entries[0].Name != "src" || !listing.Entries[0].IsDir || listing.Entries[1].Name != ".env" {
			t.Fatalf("listing: %+v", listing)
		}
	})
	t.Run("search", func(t *testing.T) {
		got := run(t, service, "search", `{"query":"app"}`)
		if got.status != cli.ExitOK || !strings.Contains(string(got.stdout), `"path":"src/app.go"`) {
			t.Fatalf("search: %d %s", got.status, got.stdout)
		}
	})
	t.Run("archive", func(t *testing.T) {
		got := run(t, service, "archive", `{"path":"src"}`)
		if got.status != cli.ExitOK {
			t.Fatalf("archive: %d %s", got.status, got.stderr)
		}
		published := publishedPath(t, got.stdout)
		if !strings.HasPrefix(published, config.DownloadsDirectory+"/") || filepath.Base(published) != "src.zip" {
			t.Fatalf("archive published at %q", published)
		}
		if names := zipEntries(t, filepath.Join(root, published)); len(names) != 1 || names[0] != "app.go" {
			t.Fatalf("archive entries: %v", names)
		}
	})
	t.Run("published archives stay out of the workspace views", func(t *testing.T) {
		got := run(t, service, "archive", `{}`)
		if got.status != cli.ExitOK {
			t.Fatalf("archive: %d %s", got.status, got.stderr)
		}
		for _, name := range zipEntries(t, filepath.Join(root, publishedPath(t, got.stdout))) {
			if strings.HasPrefix(name, config.DownloadsDirectory) {
				t.Fatalf("workspace archive includes %s", name)
			}
		}
		if listed := run(t, service, "list", `{}`); strings.Contains(string(listed.stdout), config.DownloadsDirectory) {
			t.Fatalf("listing shows downloads: %s", listed.stdout)
		}
		if found := run(t, service, "search", `{"query":"zip"}`); strings.Contains(string(found.stdout), config.DownloadsDirectory) {
			t.Fatalf("search shows downloads: %s", found.stdout)
		}
	})
	for _, test := range []struct {
		name, command, request string
		status                 int
	}{
		{"missing folder", "list", `{"path":"missing"}`, cli.ExitNotFound},
		{"traversal", "list", `{"path":"../.."}`, cli.ExitOK},
		{"missing archive", "archive", `{"path":"missing"}`, cli.ExitNotFound},
		{"unknown command", "download", `{}`, cli.ExitUsage},
		{"malformed request", "list", `{`, cli.ExitUsage},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := run(t, service, test.command, test.request); got.status != test.status {
				t.Fatalf("status %d, want %d: %s", got.status, test.status, got.stderr)
			}
		})
	}
	t.Run("traversal stays in the workspace", func(t *testing.T) {
		got := run(t, service, "list", `{"path":"../.."}`)
		if strings.Contains(string(got.stdout), "outside-secret") {
			t.Fatalf("listing escaped the workspace: %s", got.stdout)
		}
	})
}

func TestArchiveFailureWritesNoOutput(t *testing.T) {
	root := setupWorkspace(t)
	got := run(t, newService(root, 4), "archive", `{"path":"src"}`)
	if got.status != cli.ExitTooLarge || len(got.stdout) != 0 {
		t.Fatalf("oversized archive: status %d, %d bytes written", got.status, len(got.stdout))
	}
}

func newService(root string, maxArchiveBytes int64) *files.Service {
	return files.New(root, workspace.New(), archives.New(root, maxArchiveBytes, config.DownloadRetention))
}

func publishedPath(t *testing.T, stdout []byte) string {
	t.Helper()
	var published struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(stdout, &published); err != nil {
		t.Fatalf("archive output %q: %v", stdout, err)
	}
	return published.Path
}

func zipEntries(t *testing.T, file string) []string {
	t.Helper()
	archive, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	names := make([]string, 0, len(archive.File))
	for _, entry := range archive.File {
		names = append(names, entry.Name)
	}
	return names
}

func setupWorkspace(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "workspace")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "app.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "outside-secret.txt"), []byte("top secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
