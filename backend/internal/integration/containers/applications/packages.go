package applications

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// An uploaded application package is a ZIP of exactly what an applications/<id>/
// directory holds. Unpacked, it is indistinguishable from an application the binary
// was built with: the same application.json, the same install.sh, the same ui/ and
// backend/ conventions, and the same loader validating all of it.
//
// It lives in the server's state directory rather than in the binary, which is
// the whole point: updating Remote replaces the program and its built-in
// catalog, and leaves this directory — with every uploaded application, every
// installed instance and every setting — untouched.
//
// The on-disk layout is chosen so the store *is* a catalog filesystem:
//
//	<root>/applications/<id>/…    the extracted package, exactly as loadApplication wants it
//	<root>/meta/<id>.json   who uploaded it, when, and from which archive
//	<root>/staging/…        half-written uploads, never visible to a reader
//
// os.DirFS(<root>) therefore loads through the same code path as the embedded
// catalog, with no second implementation to keep in step.
const (
	packageApplicationsDir = catalogRoot
	packageMetaDir         = "meta"
	packageStagingDir      = "staging"
)

// PackageStore keeps uploaded application packages on disk and exposes them as
// a catalog filesystem.
type PackageStore struct {
	root string
	// mu serializes writers. A catalog reader goes through FS on the
	// committed application directories, which a writer only ever replaces by
	// rename, so it never observes a half-written package. list is the
	// exception: it reads the directory and the metadata files unlocked, and
	// those are written in place, so a listing taken during an upload can
	// show a package's previous metadata or none at all.
	mu sync.Mutex
	// now is injectable so tests can pin upload timestamps.
	now func() time.Time
}

// NewPackageStore prepares the store rooted at dir, creating it if needed.
func NewPackageStore(dir string) (*PackageStore, error) {
	store := &PackageStore{root: dir, now: time.Now}
	for _, sub := range []string{packageApplicationsDir, packageMetaDir, packageStagingDir} {
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
// applications/<id>/ shape the embedded catalog has.
func (s *PackageStore) FS() fs.FS { return os.DirFS(s.root) }

// add validates an uploaded archive and publishes it, replacing any earlier
// upload with the same id.
//
// reserve is consulted as soon as the id is known and before anything is
// written, so an id the catalog refuses cannot displace a package already
// stored under that name.
//
// Nothing reaches the live catalog until the archive has been unpacked into a
// staging directory *and* loaded by the same validator the built-in catalog
// goes through. An upload that would not have produced a working application
// leaves the previous one exactly as it was.
func (s *PackageStore) add(
	upload svc.PackageUpload,
	reserve func(string) error,
) (svc.PackageMutation, error) {
	files, err := readPackageArchive(upload.Data)
	if err != nil {
		return svc.PackageMutation{}, err
	}
	id, err := packageID(files)
	if err != nil {
		return svc.PackageMutation{}, err
	}
	if err := reserve(id); err != nil {
		return svc.PackageMutation{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	staged, err := os.MkdirTemp(filepath.Join(s.root, packageStagingDir), "upload-")
	if err != nil {
		return svc.PackageMutation{}, fmt.Errorf("stage package: %w", err)
	}
	defer os.RemoveAll(staged)

	stagedDir := filepath.Join(staged, packageApplicationsDir, id)
	if err := writePackageFiles(stagedDir, files); err != nil {
		return svc.PackageMutation{}, err
	}
	// The staged tree is laid out as a catalog of one, so the package is
	// validated by loadApplication itself — not by a parallel set of checks that
	// could drift from what the server will actually accept at install time.
	application, _, err := loadApplication(os.DirFS(staged), id)
	if err != nil {
		return svc.PackageMutation{}, fmt.Errorf("%w: %s", svc.ErrPackageInvalid, err)
	}

	replaced, err := s.publish(id, stagedDir)
	if err != nil {
		return svc.PackageMutation{}, err
	}

	digest := sha256.Sum256(upload.Data)
	pkg := svc.Package{
		ID:         id,
		Name:       application.Name,
		Version:    application.Version,
		Scopes:     application.Scopes,
		Filename:   filepath.Base(filepath.Clean(upload.Filename)),
		Size:       int64(len(upload.Data)),
		SHA256:     hex.EncodeToString(digest[:]),
		UploadedAt: s.now().Unix(),
		UploadedBy: upload.Actor,
	}
	if err := s.writeMeta(pkg); err != nil {
		return svc.PackageMutation{}, err
	}
	return svc.PackageMutation{Package: pkg, Replaced: replaced}, nil
}

// publish swaps a staged application directory into the committed catalog. The
// previous copy is moved aside first and only deleted once the new one is in
// place, so a failure mid-swap restores what was there rather than leaving the
// id with no directory at all.
func (s *PackageStore) publish(id, stagedDir string) (bool, error) {
	live := filepath.Join(s.root, packageApplicationsDir, id)
	previous := live + ".replaced"
	_ = os.RemoveAll(previous)

	hadPrevious := false
	if _, err := os.Stat(live); err == nil {
		if err := os.Rename(live, previous); err != nil {
			return false, fmt.Errorf("replace package: %w", err)
		}
		hadPrevious = true
	}
	if err := os.Rename(stagedDir, live); err != nil {
		if hadPrevious {
			_ = os.Rename(previous, live)
		}
		return false, fmt.Errorf("publish package: %w", err)
	}
	if hadPrevious {
		_ = os.RemoveAll(previous)
	}
	return hadPrevious, nil
}

// remove deletes a stored package and its metadata.
func (s *PackageStore) remove(id string) error {
	if !packageIDPattern.MatchString(id) {
		return svc.ErrPackageNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	live := filepath.Join(s.root, packageApplicationsDir, id)
	if _, err := os.Stat(live); errors.Is(err, fs.ErrNotExist) {
		return svc.ErrPackageNotFound
	}
	if err := os.RemoveAll(live); err != nil {
		return fmt.Errorf("remove package: %w", err)
	}
	_ = os.Remove(s.metaPath(id))
	return nil
}

// list returns every stored package, newest upload first. A package whose
// directory exists but whose metadata does not is still listed: the catalog
// entry it produces is real, and hiding it would make it unremovable.
func (s *PackageStore) list() []svc.Package {
	entries, err := os.ReadDir(filepath.Join(s.root, packageApplicationsDir))
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

// writePackageFiles materializes the package under dir. Nothing is written
// executable: an install script is piped into bash by the installer and a
// application is compiled from source, so no file in a package is ever run
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
