package appplugin

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The embedded SDK is what plugins compile against. A new file that is not in
// the embed list would compile in this repository and fail on the server, so
// the completeness of that list is the invariant worth pinning.
func TestSourceCoversEveryFile(t *testing.T) {
	embedded := map[string]bool{}
	if err := fs.WalkDir(Source(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		embedded[path] = true
		return nil
	}); err != nil {
		t.Fatalf("walk embedded source: %v", err)
	}

	err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		name := filepath.ToSlash(path)
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		if !embedded[name] {
			t.Errorf("%s is not in the //go:embed list in source.go", name)
		}
		delete(embedded, name)
		return nil
	})
	if err != nil {
		t.Fatalf("walk package: %v", err)
	}
	for name := range embedded {
		t.Errorf("%s is embedded but does not exist", name)
	}
}
