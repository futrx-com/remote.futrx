package pluginhost

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

type sourceFile struct {
	path string
	data []byte
}

// collect reads a source tree into a deterministic, sorted list. Determinism
// is the point: the same tree must always produce the same fingerprint.
func collect(fsys fs.FS) ([]sourceFile, error) {
	var files []sourceFile
	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		files = append(files, sourceFile{path: path.Clean(name), data: data})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	return files, nil
}

func writeAll(root string, files []sourceFile) error {
	for _, file := range files {
		if err := writeFile(filepath.Join(root, filepath.FromSlash(file.path)), file.data); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(name string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(name), err)
	}
	if err := os.WriteFile(name, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// fingerprintOf hashes everything that can change a plugin binary: its source,
// the SDK it links, the module files pinning their dependencies, and the Go
// version. Anything left out here would be a stale binary someone has to
// diagnose, so the inputs are hashed whole rather than by modification time.
func fingerprintOf(files, sdk []sourceFile, pluginModule, sdkModule, goVersion string) string {
	digest := sha256.New()
	writeSection := func(label string, section []sourceFile) {
		writeChunk(digest, []byte(label))
		for _, file := range section {
			writeChunk(digest, []byte(file.path))
			writeChunk(digest, file.data)
		}
	}
	writeSection("plugin", files)
	writeSection("sdk", sdk)
	writeChunk(digest, []byte(pluginModule))
	writeChunk(digest, []byte(sdkModule))
	writeChunk(digest, []byte(goVersion))
	return hex.EncodeToString(digest.Sum(nil))[:16]
}

// writeChunk length-prefixes each value so that concatenating two different
// splits of the same bytes cannot collide.
func writeChunk(digest interface{ Write([]byte) (int, error) }, value []byte) {
	var length [8]byte
	binary.LittleEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = digest.Write(length[:])
	_, _ = digest.Write(value)
}
