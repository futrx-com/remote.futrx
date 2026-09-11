package hosttools

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func gzipped(t *testing.T, payload []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// testInstaller serves one artifact from memory and records how often it was
// asked for, so "downloaded again" is distinguishable from "already present".
func testInstaller(t *testing.T, artifact []byte) (*Installer, *int) {
	t.Helper()
	downloads := 0
	in := New(t.TempDir())
	in.arch = "testarch"
	in.fetch = func(context.Context, string) (io.ReadCloser, error) {
		downloads++
		return io.NopCloser(bytes.NewReader(artifact)), nil
	}
	in.verify = func(_ context.Context, path string, _ []string) error {
		if _, err := os.Stat(path); err != nil {
			return err
		}
		return nil
	}
	return in, &downloads
}

func tool(artifact []byte, compression string) svc.HostTool {
	return svc.HostTool{
		Name:    "backup-cli",
		Version: "1.2.3",
		Downloads: map[string]svc.HostToolDownload{
			"testarch": {URL: "https://example.invalid/backup-cli", SHA256: digest(artifact), Compression: compression},
		},
	}
}

// The whole point of a declared host tool is that Remote installs exactly what
// the image asked for and can then find it again — without a package manager
// and without touching anything outside its own data directory.
func TestEnsureInstallsDecompressesAndPublishes(t *testing.T) {
	binary := []byte("#!/bin/sh\necho backup\n")
	artifact := gzipped(t, binary)
	in, downloads := testInstaller(t, artifact)

	if err := in.Ensure(context.Background(), []svc.HostTool{tool(artifact, "gzip")}); err != nil {
		t.Fatal(err)
	}
	path, err := Lookup(in.dataDir, "backup-cli")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, binary) {
		t.Fatalf("installed contents = %q, want the decompressed binary", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Errorf("mode = %v, want an executable", info.Mode())
	}
	// The versioned path behind the published name is what makes a later
	// version install beside this one instead of overwriting a binary a running
	// backup is still using.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(Dir(in.dataDir), "backup-cli", "1.2.3", "backup-cli"))
	if err != nil {
		t.Fatal(err)
	}
	if resolved != want {
		t.Errorf("%q resolves to %q, want the version-scoped %q", path, resolved, want)
	}

	// A second Ensure re-verifies but must not re-download.
	if err := in.Ensure(context.Background(), []svc.HostTool{tool(artifact, "gzip")}); err != nil {
		t.Fatal(err)
	}
	if *downloads != 1 {
		t.Errorf("downloads = %d, want the installed copy reused", *downloads)
	}
}

func TestEnsureSupportsBzip2AndUncompressedArtifacts(t *testing.T) {
	binary := []byte("binary contents\n")

	t.Run("uncompressed", func(t *testing.T) {
		in, _ := testInstaller(t, binary)
		if err := in.Ensure(context.Background(), []svc.HostTool{tool(binary, "")}); err != nil {
			t.Fatal(err)
		}
	})

	// compress/bzip2 only decompresses, so the fixture is a known-good archive
	// rather than one produced here.
	t.Run("bzip2", func(t *testing.T) {
		artifact, err := os.ReadFile("testdata/binary.bz2")
		if err != nil {
			t.Skipf("no bzip2 fixture: %v", err)
		}
		in, _ := testInstaller(t, artifact)
		if err := in.Ensure(context.Background(), []svc.HostTool{tool(artifact, "bzip2")}); err != nil {
			t.Fatal(err)
		}
		path, err := Lookup(in.dataDir, "backup-cli")
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		want, err := io.ReadAll(bzip2.NewReader(bytes.NewReader(artifact)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("installed contents = %q, want %q", got, want)
		}
	})
}

// A checksum that is not enforced is decoration. Serving different bytes than
// the image declared must leave nothing installed at all.
func TestEnsureRejectsAMismatchedChecksum(t *testing.T) {
	declared := []byte("what the image pinned\n")
	in, _ := testInstaller(t, []byte("what the network served\n"))

	err := in.Ensure(context.Background(), []svc.HostTool{tool(declared, "")})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v, want a checksum mismatch", err)
	}
	if _, err := Lookup(in.dataDir, "backup-cli"); err == nil {
		t.Error("a tool that failed its checksum must not be installed")
	}
}

// A binary that downloads but will not run is not installed: reporting the
// image as installed would defer the failure to the first backup.
func TestEnsureRejectsABinaryThatWillNotRun(t *testing.T) {
	artifact := []byte("binary\n")
	in, _ := testInstaller(t, artifact)
	in.verify = func(context.Context, string, []string) error { return errors.New("exec format error") }

	if err := in.Ensure(context.Background(), []svc.HostTool{tool(artifact, "")}); err == nil {
		t.Fatal("accepted a binary that does not run")
	}
	if _, err := Lookup(in.dataDir, "backup-cli"); err == nil {
		t.Error("an unverified tool must not be published")
	}
}

func TestEnsureRejectsAnUnavailableArchitecture(t *testing.T) {
	artifact := []byte("binary\n")
	in, _ := testInstaller(t, artifact)
	in.arch = "s390x"

	err := in.Ensure(context.Background(), []svc.HostTool{tool(artifact, "")})
	if err == nil || !strings.Contains(err.Error(), "s390x") {
		t.Fatalf("err = %v, want the host architecture named", err)
	}
}

// Validate runs at catalog load, so a malformed declaration fails when the
// image is read rather than on the host of whoever installs it first.
func TestValidateRejectsUnusableDeclarations(t *testing.T) {
	sum := digest([]byte("x"))
	ok := func() svc.HostTool {
		return svc.HostTool{
			Name:      "backup-cli",
			Version:   "1.2.3",
			Downloads: map[string]svc.HostToolDownload{"amd64": {URL: "https://example.invalid/x", SHA256: sum}},
		}
	}
	if err := Validate(ok()); err != nil {
		t.Fatalf("rejected a valid tool: %v", err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*svc.HostTool)
	}{
		{"no name", func(h *svc.HostTool) { h.Name = "" }},
		{"a path for a name", func(h *svc.HostTool) { h.Name = "../../bin/sh" }},
		{"a hidden name", func(h *svc.HostTool) { h.Name = ".ssh" }},
		{"no version", func(h *svc.HostTool) { h.Version = "" }},
		{"a path for a version", func(h *svc.HostTool) { h.Version = "../etc" }},
		{"no downloads", func(h *svc.HostTool) { h.Downloads = nil }},
		{"a plaintext download", func(h *svc.HostTool) {
			h.Downloads["amd64"] = svc.HostToolDownload{URL: "http://example.invalid/x", SHA256: sum}
		}},
		{"a download carrying credentials", func(h *svc.HostTool) {
			h.Downloads["amd64"] = svc.HostToolDownload{URL: "https://user:pass@example.invalid/x", SHA256: sum}
		}},
		{"no checksum", func(h *svc.HostTool) {
			h.Downloads["amd64"] = svc.HostToolDownload{URL: "https://example.invalid/x"}
		}},
		{"a truncated checksum", func(h *svc.HostTool) {
			h.Downloads["amd64"] = svc.HostToolDownload{URL: "https://example.invalid/x", SHA256: sum[:32]}
		}},
		{"unsupported compression", func(h *svc.HostTool) {
			h.Downloads["amd64"] = svc.HostToolDownload{URL: "https://example.invalid/x", SHA256: sum, Compression: "xz"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := ok()
			tc.mutate(&h)
			if err := Validate(h); err == nil {
				t.Error("want a validation error, got nil")
			}
		})
	}
}

// A host that never installs such an image has nothing to look up, and must be
// told so rather than silently falling back to something unpinned.
func TestLookupReportsAToolThatWasNeverInstalled(t *testing.T) {
	if _, err := Lookup(t.TempDir(), "definitely-not-a-real-binary"); err == nil {
		t.Fatal("want an error for a tool nobody installed")
	}
}
