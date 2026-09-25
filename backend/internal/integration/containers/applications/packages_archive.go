package applications

// Reading an uploaded package archive. Nothing here touches the filesystem:
// it turns the bytes of a ZIP into the files an application directory is made
// of, and refuses anything that could not become one. The store above writes
// whatever survives.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/config/constants"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// Limits on an uploaded archive. They bound what a single admin request can
// make the server write and read before the catalog loader ever sees it.
const (
	// maxPackageExpanded caps the total size of everything unpacked, which is
	// what a compression bomb would otherwise blow past.
	maxPackageExpanded = 192 << 20
	// maxPackageFile caps any single member.
	maxPackageFile = 64 << 20
	// maxPackageEntries caps the member count, bounding path handling and
	// inode use independently of size.
	maxPackageEntries = 8192
)

// packageIDPattern is what an application id may look like. It is deliberately
// narrower than a filename: the id becomes a directory name, a URL path
// segment, a Go build directory and a container-facing identifier, so anything
// needing escaping anywhere is refused once, here.
var packageIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// readPackageArchive decodes a ZIP into the files it carries, keyed by their
// path inside the application directory.
//
// It accepts both shapes people actually produce: an archive whose root *is*
// the application directory, and one holding a single folder that is the application
// directory — which is what every desktop "compress this folder" produces.
func readPackageArchive(data []byte) (map[string][]byte, error) {
	if len(data) > constants.MaxApplicationPackageArchiveBytes {
		return nil, fmt.Errorf(
			"%w: the archive is larger than %d MiB",
			svc.ErrPackageInvalid, constants.MaxApplicationPackageArchiveBytes>>20)
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%w: not a readable ZIP archive (%s)", svc.ErrPackageInvalid, err)
	}
	if len(reader.File) > maxPackageEntries {
		return nil, fmt.Errorf(
			"%w: the archive holds more than %d files", svc.ErrPackageInvalid, maxPackageEntries)
	}

	names := make([]string, 0, len(reader.File))
	members := make([]*zip.File, 0, len(reader.File))
	for _, entry := range reader.File {
		name, keep, err := packageEntryName(entry)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		names = append(names, name)
		members = append(members, entry)
	}
	prefix, err := packageRootPrefix(names)
	if err != nil {
		return nil, err
	}

	files := make(map[string][]byte, len(members))
	var total int64
	for i, entry := range members {
		name := strings.TrimPrefix(names[i], prefix)
		if name == "" {
			continue
		}
		if _, duplicate := files[name]; duplicate {
			return nil, fmt.Errorf("%w: %q appears twice in the archive", svc.ErrPackageInvalid, name)
		}
		content, err := readPackageEntry(entry, name)
		if err != nil {
			return nil, err
		}
		total += int64(len(content))
		if total > maxPackageExpanded {
			return nil, fmt.Errorf(
				"%w: the archive unpacks to more than %d MiB",
				svc.ErrPackageInvalid, maxPackageExpanded>>20)
		}
		files[name] = content
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: the archive contains no files", svc.ErrPackageInvalid)
	}
	return files, nil
}

// packageEntryName normalizes one archive member's path and decides whether it
// is part of the package at all. Directory entries and archiver bookkeeping
// carry nothing, so they are dropped rather than rejected; anything that could
// escape the application directory, or that is not a plain file, is rejected.
func packageEntryName(entry *zip.File) (name string, keep bool, err error) {
	raw := strings.ReplaceAll(entry.Name, `\`, "/")
	if strings.HasSuffix(raw, "/") {
		return "", false, nil
	}
	clean := path.Clean(raw)
	if !fs.ValidPath(clean) || clean == "." || strings.HasPrefix(clean, "../") {
		return "", false, fmt.Errorf("%w: unsafe path %q in the archive", svc.ErrPackageInvalid, entry.Name)
	}
	if isArchiveNoise(clean) {
		return "", false, nil
	}
	mode := entry.Mode()
	if mode.IsDir() {
		return "", false, nil
	}
	if !mode.IsRegular() {
		return "", false, fmt.Errorf(
			"%w: %q is a symlink or special file; a package may contain only regular files",
			svc.ErrPackageInvalid, entry.Name)
	}
	if entry.UncompressedSize64 > maxPackageFile {
		return "", false, fmt.Errorf(
			"%w: %q is larger than %d MiB", svc.ErrPackageInvalid, entry.Name, maxPackageFile>>20)
	}
	return clean, true, nil
}

// isArchiveNoise matches the bookkeeping desktop archivers add. Keeping it
// would fail the "one top-level directory" check and litter the application
// directory with files no application ever declares.
func isArchiveNoise(name string) bool {
	if name == "__MACOSX" || strings.HasPrefix(name, "__MACOSX/") {
		return true
	}
	base := path.Base(name)
	return base == ".DS_Store" || base == "Thumbs.db" || strings.HasPrefix(base, "._")
}

// packageRootPrefix finds the prefix to strip so application.json lands at the root.
func packageRootPrefix(names []string) (string, error) {
	for _, name := range names {
		if name == "application.json" {
			return "", nil
		}
	}
	// No application.json at the root: accept a single wrapping directory, which is
	// what compressing the application folder itself produces.
	root := ""
	for _, name := range names {
		top, _, nested := strings.Cut(name, "/")
		if !nested {
			return "", missingManifest()
		}
		if root == "" {
			root = top
			continue
		}
		if root != top {
			return "", missingManifest()
		}
	}
	if root == "" {
		return "", missingManifest()
	}
	for _, name := range names {
		if name == root+"/application.json" {
			return root + "/", nil
		}
	}
	return "", missingManifest()
}

func missingManifest() error {
	return fmt.Errorf(
		"%w: no application.json found — the archive must hold the application's files at its root, "+
			"or inside a single folder", svc.ErrPackageInvalid)
}

func readPackageEntry(entry *zip.File, name string) ([]byte, error) {
	source, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read %q (%s)", svc.ErrPackageInvalid, name, err)
	}
	defer source.Close()
	// The declared size is a claim; reading one byte past the cap is what
	// catches an archive whose header understates what it actually holds.
	limited := &io.LimitedReader{R: source, N: maxPackageFile + 1}
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read %q (%s)", svc.ErrPackageInvalid, name, err)
	}
	if int64(len(content)) > maxPackageFile {
		return nil, fmt.Errorf(
			"%w: %q is larger than %d MiB", svc.ErrPackageInvalid, name, maxPackageFile>>20)
	}
	return content, nil
}

// packageID reads the id the package claims. It comes from application.json and
// nowhere else: deriving it from the uploaded filename would let the same
// package install under two ids depending on what the browser called the file.
func packageID(files map[string][]byte) (string, error) {
	raw, ok := files["application.json"]
	if !ok {
		return "", missingManifest()
	}
	var manifest struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("%w: application.json is not valid JSON (%s)", svc.ErrPackageInvalid, err)
	}
	id := strings.TrimSpace(manifest.ID)
	if id == "" {
		return "", fmt.Errorf(
			`%w: application.json must set "id" — it is the application's permanent identity`,
			svc.ErrPackageInvalid)
	}
	if !packageIDPattern.MatchString(id) {
		return "", fmt.Errorf(
			"%w: id %q must be lowercase letters, digits and dashes, starting with a letter or digit",
			svc.ErrPackageInvalid, id)
	}
	if isCatalogMetadataDirectory(id) {
		return "", fmt.Errorf("%w: id %q is reserved", svc.ErrPackageInvalid, id)
	}
	return id, nil
}
