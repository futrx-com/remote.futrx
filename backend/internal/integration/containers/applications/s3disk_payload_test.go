package applications

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"testing"
)

// s3disk ships its mount as container/, a nested Go module. go:embed does not
// traverse one and does not complain when it skips it, so the module reaches
// the binary only as the container.tar.gz package.sh builds. Nothing else
// would notice that archive going stale: the build stays green and the release
// ships whatever mount source was current the last time somebody remembered to
// run the script. This is what notices.
//
// The file set below is the one package.sh packs. They are two statements of
// the same rule, so a change to either belongs in both.
func TestS3diskPayloadMatchesContainerSource(t *testing.T) {
	const image = "images/s3disk"

	archive, err := fs.ReadFile(catalogFS, path.Join(image, "container.tar.gz"))
	if err != nil {
		t.Fatalf("read embedded payload: %v", err)
	}
	packed := map[string][]byte{}
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatalf("gunzip payload: %v", err)
	}
	tr := tar.NewReader(zr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read payload: %v", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read %s from payload: %v", header.Name, err)
		}
		packed[header.Name] = content
	}

	// container/ is not embedded, so the source it should match is read from
	// disk. Tests run in their package directory, which is what this is
	// relative to.
	onDisk := map[string][]byte{}
	root := path.Join(image, "container")
	err = fs.WalkDir(os.DirFS("."), root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		base := path.Base(name)
		if !strings.HasSuffix(base, ".go") && base != "go.mod" && base != "go.sum" && base != "VERSION" {
			return nil
		}
		content, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		onDisk[name] = content
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	if len(onDisk) == 0 {
		t.Fatalf("no container source found under %s", root)
	}

	// Paths in the archive are relative to the image directory.
	const fix = "run: " + image + "/package.sh"
	for name, want := range onDisk {
		rel := strings.TrimPrefix(name, image+"/")
		got, ok := packed[rel]
		if !ok {
			t.Errorf("%s is missing from container.tar.gz — %s", name, fix)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from the copy in container.tar.gz — %s", name, fix)
		}
		delete(packed, rel)
	}
	for rel := range packed {
		t.Errorf("container.tar.gz carries %s, which no longer exists — %s", rel, fix)
	}
}
