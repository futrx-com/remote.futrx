package applications

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"os/exec"
	"strings"
	"testing"
	"testing/fstest"
)

func testPayload(t *testing.T, name string, kind byte) []byte {
	t.Helper()
	var out bytes.Buffer
	compressed := gzip.NewWriter(&out)
	archive := tar.NewWriter(compressed)
	content := []byte("bundled source\n")
	h := &tar.Header{Name: name, Typeflag: kind, Mode: 0644}
	if kind == tar.TypeReg {
		h.Size = int64(len(content))
	} else {
		h.Linkname = "/etc/passwd"
	}
	if err := archive.WriteHeader(h); err != nil {
		t.Fatal(err)
	}
	if kind == tar.TypeReg {
		if _, err := archive.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestContainerPayloadStagesSourceAndCleansUp(t *testing.T) {
	files := fstest.MapFS{"image/container.tar.gz": {Data: testPayload(t, "container/go.mod", tar.TypeReg)}}
	script := []byte("printf '%s\\n' \"$APP_PACKAGE_DIR\"\ncat \"$APP_PACKAGE_DIR/container/go.mod\"\nexit 7\n")
	wrapped, err := withContainerPayload(files, "image", script)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-s")
	cmd.Stdin = bytes.NewReader(wrapped)
	output, err := cmd.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 {
		t.Fatalf("exit: %v, output %s", err, output)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 2 || lines[1] != "bundled source" {
		t.Fatalf("output: %s", output)
	}
	if _, err := os.Stat(lines[0]); !os.IsNotExist(err) {
		t.Fatalf("staging directory was not removed: %v", err)
	}
}

func TestContainerPayloadRejectsUnsafeArchives(t *testing.T) {
	for _, name := range []string{"../escape", "container/../../escape", "/container/file", "plugin/main.go", "container\\escape"} {
		if err := validateContainerPayload(testPayload(t, name, tar.TypeReg)); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for _, kind := range []byte{tar.TypeSymlink, tar.TypeLink} {
		if err := validateContainerPayload(testPayload(t, "container/link", kind)); err == nil {
			t.Errorf("accepted link kind %d", kind)
		}
	}
	payload := testPayload(t, "container/go.mod", tar.TypeReg)
	payload[len(payload)-8] ^= 0xff
	if err := validateContainerPayload(payload); err == nil {
		t.Error("accepted corrupt gzip checksum")
	}
}

func TestContainerPayloadLeavesLegacyScriptsUnchanged(t *testing.T) {
	original := []byte("echo original\n")
	got, err := withContainerPayload(fstest.MapFS{}, "image", original)
	if err != nil || !bytes.Equal(got, original) {
		t.Fatalf("legacy script changed: %s, %v", got, err)
	}
}
