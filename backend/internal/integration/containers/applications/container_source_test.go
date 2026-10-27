package applications

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

func TestPackContainerSourceIsDeterministicAndSynthesizesModule(t *testing.T) {
	files := fstest.MapFS{
		"applications/example/backend/container/cmd/worker/main.go":    {Data: []byte("package main\nfunc main() {}\n")},
		"applications/example/backend/container/internal/info/info.go": {Data: []byte("package info\n")},
	}
	one, err := packContainerSource(files, "applications/example", "example")
	if err != nil {
		t.Fatal(err)
	}
	two, err := packContainerSource(files, "applications/example", "example")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(one.payload, two.payload) || one.digest != two.digest {
		t.Fatal("unchanged container source did not pack deterministically")
	}
	if !reflect.DeepEqual(one.commands, []string{"worker"}) {
		t.Fatalf("commands = %v, want worker", one.commands)
	}

	packed := unpackTestPayload(t, one.payload)
	module := string(packed["infra/go.mod"])
	if !strings.Contains(module, "module futrx.local/catalog/applications/example/backend/container") ||
		!strings.Contains(module, "go "+configconstants.ApplicationContainerGoVersion) {
		t.Fatalf("synthesized go.mod = %q", module)
	}
	if string(packed["infra/cmd/worker/main.go"]) == "" {
		t.Fatal("container command source was not packed")
	}
}

func TestPackContainerSourceKeepsUploadedModule(t *testing.T) {
	files := fstest.MapFS{
		"applications/example/backend/container/go.mod":  {Data: []byte("module example.com/custom\n\ngo 1.24\n")},
		"applications/example/backend/container/main.go": {Data: []byte("package main\nfunc main() {}\n")},
	}
	packed, err := packContainerSource(files, "applications/example", "example")
	if err != nil {
		t.Fatal(err)
	}
	contents := unpackTestPayload(t, packed.payload)
	if got := string(contents["infra/go.mod"]); !strings.Contains(got, "example.com/custom") {
		t.Fatalf("go.mod = %q", got)
	}
}

func unpackTestPayload(t *testing.T, payload []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	files := map[string][]byte{}
	archive := tar.NewReader(gz)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return files
		}
		if err != nil {
			t.Fatal(err)
		}
		contents, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		files[header.Name] = contents
	}
}
