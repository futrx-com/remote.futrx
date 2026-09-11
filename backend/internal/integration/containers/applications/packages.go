package applications

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// An uploaded application package is a ZIP of exactly what an images/<id>/
// directory holds. Unpacked, it is indistinguishable from an image the binary
// was built with: the same image.json, the same install.sh, the same ui/ and
// plugin/ conventions, and the same loader validating all of it.
//
// It lives in the server's state directory rather than in the binary, which is
// the whole point: updating Remote replaces the program and its built-in
// catalog, and leaves this directory — with every uploaded application, every
// installed instance and every setting — untouched.
//
// The on-disk layout is chosen so the store *is* a catalog filesystem:
//
//	<root>/images/<id>/…    the extracted package, exactly as loadImage wants it
//	<root>/meta/<id>.json   who uploaded it, when, and from which archive
//	<root>/staging/…        half-written uploads, never visible to a reader
//
// os.DirFS(<root>) therefore loads through the same code path as the embedded
// catalog, with no second implementation to keep in step.
const (
	packageImagesDir  = catalogRoot
	packageMetaDir    = "meta"
	packageStagingDir = "staging"
)

// Limits on an uploaded archive. They bound what a single admin request can
// make the server write and read before the catalog loader ever sees it.
const (
	// maxPackageArchive caps the ZIP itself.
	maxPackageArchive = 64 << 20
	// maxPackageExpanded caps the total size of everything unpacked, which is
	// what a compression bomb would otherwise blow past.
	maxPackageExpanded = 192 << 20
	// maxPackageFile caps any single member.
	maxPackageFile = 64 << 20
	// maxPackageEntries caps the member count, bounding path handling and
	// inode use independently of size.
	maxPackageEntries = 8192
)

// packageIDPattern is what an image id may look like. It is deliberately
// narrower than a filename: the id becomes a directory name, a URL path
// segment, a Go build directory and a container-facing identifier, so anything
// needing escaping anywhere is refused once, here.
var packageIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// PackageStore keeps uploaded application packages on disk and exposes them as
// a catalog filesystem.
type PackageStore struct {
	root string
	// mu serializes writers. Readers go through fs.FS on the committed
	// directory, which a writer only ever replaces by rename.
	mu sync.Mutex
	// now is injectable so tests can pin upload timestamps.
	now func() time.Time
}

// NewPackageStore prepares the store rooted at dir, creating it if needed.
func NewPackageStore(dir string) (*PackageStore, error) {
	store := &PackageStore{root: dir, now: time.Now}
	for _, sub := range []string{packageImagesDir, packageMetaDir, packageStagingDir} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return nil, fmt.Errorf("prepare package store: %w", err)
		}
	}
	// A staging directory left behind by a crash holds no committed state, so
	// clearing it on start is safe and keeps failed uploads from accumulating.
	store.clearStaging()
	return store, nil
}

// FS exposes the committed packages as a catalog filesystem, with the same
// images/<id>/ shape the embedded catalog has.
func (s *PackageStore) FS() fs.FS { return os.DirFS(s.root) }

// install validates an uploaded archive and publishes it, replacing any earlier
// upload with the same id.
//
// accept is consulted as soon as the id is known and before anything is
// written, so an id the catalog refuses cannot displace a package already
// stored under that name.
//
// Nothing reaches the live catalog until the archive has been unpacked into a
// staging directory *and* loaded by the same validator the built-in catalog
// goes through. An upload that would not have produced a working application
// leaves the previous one exactly as it was.
func (s *PackageStore) install(upload svc.PackageUpload, accept func(string) error) (svc.Package, error) {
	if len(upload.Data) > maxPackageArchive {
		return svc.Package{}, fmt.Errorf(
			"%w: the archive is larger than %d MiB", svc.ErrPackageInvalid, maxPackageArchive>>20)
	}
	files, err := readPackageArchive(upload.Data)
	if err != nil {
		return svc.Package{}, err
	}
	id, err := packageID(files)
	if err != nil {
		return svc.Package{}, err
	}
	if accept != nil {
		if err := accept(id); err != nil {
			return svc.Package{}, err
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	staged, err := os.MkdirTemp(filepath.Join(s.root, packageStagingDir), "upload-")
	if err != nil {
		return svc.Package{}, fmt.Errorf("stage package: %w", err)
	}
	defer os.RemoveAll(staged)

	stagedImage := filepath.Join(staged, packageImagesDir, id)
	if err := writePackageFiles(stagedImage, files); err != nil {
		return svc.Package{}, err
	}
	// The staged tree is laid out as a catalog of one, so the package is
	// validated by loadImage itself — not by a parallel set of checks that
	// could drift from what the server will actually accept at install time.
	img, _, err := loadImage(os.DirFS(staged), id)
	if err != nil {
		return svc.Package{}, fmt.Errorf("%w: %s", svc.ErrPackageInvalid, err)
	}

	if err := s.publish(id, stagedImage); err != nil {
		return svc.Package{}, err
	}

	digest := sha256.Sum256(upload.Data)
	pkg := svc.Package{
		ID:         id,
		Name:       img.Name,
		Version:    img.Version,
		Type:       img.Type,
		Scopes:     img.Scopes,
		Filename:   filepath.Base(filepath.Clean(upload.Filename)),
		Size:       int64(len(upload.Data)),
		SHA256:     hex.EncodeToString(digest[:]),
		UploadedAt: s.now().Unix(),
		UploadedBy: upload.Actor,
	}
	if err := s.writeMeta(pkg); err != nil {
		return svc.Package{}, err
	}
	return pkg, nil
}

// publish swaps a staged image directory into the committed catalog. The
// previous copy is moved aside first and only deleted once the new one is in
// place, so a failure mid-swap restores what was there rather than leaving the
// id with no directory at all.
func (s *PackageStore) publish(id, stagedImage string) error {
	live := filepath.Join(s.root, packageImagesDir, id)
	previous := live + ".replaced"
	_ = os.RemoveAll(previous)

	hadPrevious := false
	if _, err := os.Stat(live); err == nil {
		if err := os.Rename(live, previous); err != nil {
			return fmt.Errorf("replace package: %w", err)
		}
		hadPrevious = true
	}
	if err := os.Rename(stagedImage, live); err != nil {
		if hadPrevious {
			_ = os.Rename(previous, live)
		}
		return fmt.Errorf("publish package: %w", err)
	}
	if hadPrevious {
		_ = os.RemoveAll(previous)
	}
	return nil
}

// RemovePackage deletes a stored package and its metadata.
func (s *PackageStore) RemovePackage(id string) error {
	if !packageIDPattern.MatchString(id) {
		return svc.ErrPackageNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	live := filepath.Join(s.root, packageImagesDir, id)
	if _, err := os.Stat(live); errors.Is(err, fs.ErrNotExist) {
		return svc.ErrPackageNotFound
	}
	if err := os.RemoveAll(live); err != nil {
		return fmt.Errorf("remove package: %w", err)
	}
	_ = os.Remove(s.metaPath(id))
	return nil
}

// Packages lists every stored package, newest upload first. A package whose
// directory exists but whose metadata does not is still listed: the catalog
// entry it produces is real, and hiding it would make it unremovable.
func (s *PackageStore) Packages() []svc.Package {
	entries, err := os.ReadDir(filepath.Join(s.root, packageImagesDir))
	if err != nil {
		return nil
	}
	out := make([]svc.Package, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !packageIDPattern.MatchString(entry.Name()) {
			continue
		}
		pkg, err := s.readMeta(entry.Name())
		if err != nil {
			pkg = svc.Package{ID: entry.Name(), Name: entry.Name()}
		}
		out = append(out, pkg)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UploadedAt != out[j].UploadedAt {
			return out[i].UploadedAt > out[j].UploadedAt
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *PackageStore) metaPath(id string) string {
	return filepath.Join(s.root, packageMetaDir, id+".json")
}

func (s *PackageStore) writeMeta(pkg svc.Package) error {
	raw, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode package metadata: %w", err)
	}
	if err := os.WriteFile(s.metaPath(pkg.ID), raw, 0o600); err != nil {
		return fmt.Errorf("write package metadata: %w", err)
	}
	return nil
}

func (s *PackageStore) readMeta(id string) (svc.Package, error) {
	raw, err := os.ReadFile(s.metaPath(id))
	if err != nil {
		return svc.Package{}, err
	}
	var pkg svc.Package
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return svc.Package{}, err
	}
	pkg.ID = id
	return pkg, nil
}

func (s *PackageStore) clearStaging() {
	staging := filepath.Join(s.root, packageStagingDir)
	entries, err := os.ReadDir(staging)
	if err != nil {
		return
	}
	for _, entry := range entries {
		_ = os.RemoveAll(filepath.Join(staging, entry.Name()))
	}
}

// ---- archive reading -------------------------------------------------------

// readPackageArchive decodes a ZIP into the files it carries, keyed by their
// path inside the image directory.
//
// It accepts both shapes people actually produce: an archive whose root *is*
// the image directory, and one holding a single folder that is the image
// directory — which is what every desktop "compress this folder" produces.
func readPackageArchive(data []byte) (map[string][]byte, error) {
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
// escape the image directory, or that is not a plain file, is rejected.
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
// would fail the "one top-level directory" check and litter the image
// directory with files no image ever declares.
func isArchiveNoise(name string) bool {
	if name == "__MACOSX" || strings.HasPrefix(name, "__MACOSX/") {
		return true
	}
	base := path.Base(name)
	return base == ".DS_Store" || base == "Thumbs.db" || strings.HasPrefix(base, "._")
}

// packageRootPrefix finds the prefix to strip so image.json lands at the root.
func packageRootPrefix(names []string) (string, error) {
	for _, name := range names {
		if name == "image.json" {
			return "", nil
		}
	}
	// No image.json at the root: accept a single wrapping directory, which is
	// what compressing the image folder itself produces.
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
		if name == root+"/image.json" {
			return root + "/", nil
		}
	}
	return "", missingManifest()
}

func missingManifest() error {
	return fmt.Errorf(
		"%w: no image.json found — the archive must hold the application's files at its root, "+
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

// packageID reads the id the package claims. It comes from image.json and
// nowhere else: deriving it from the uploaded filename would let the same
// package install under two ids depending on what the browser called the file.
func packageID(files map[string][]byte) (string, error) {
	raw, ok := files["image.json"]
	if !ok {
		return "", missingManifest()
	}
	var manifest struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", fmt.Errorf("%w: image.json is not valid JSON (%s)", svc.ErrPackageInvalid, err)
	}
	id := strings.TrimSpace(manifest.ID)
	if id == "" {
		return "", fmt.Errorf(
			`%w: image.json must set "id" — it is the application's permanent identity`,
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

// writePackageFiles materializes the package under dir. Nothing is written
// executable: an install script is piped into bash by the installer and a
// plugin is compiled from source, so no file in a package is ever run
// directly, and granting the bit would only widen what an upload can do.
func writePackageFiles(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("stage package: %w", err)
	}
	for name, content := range files {
		target := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("stage package: %w", err)
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return fmt.Errorf("stage package: %w", err)
		}
	}
	return nil
}
