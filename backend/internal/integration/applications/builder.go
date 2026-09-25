package applications

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// Builder turns an application's backend/ composition root and its host
// packages into an executable, caching the result by a fingerprint of
// everything that went into it.
//
// The catalog ships source rather than binaries because it is embedded in the
// server and has to stay portable across the architectures a server runs on.
// Compiling once per application build and caching by fingerprint keeps the cost off
// every install: a cold build takes seconds, a warm one is a stat.
type Builder struct {
	root   string
	goTool string
	pins   modulePins
	locks  keyedLocks
	// shared holds the half of every fingerprint that no application can change.
	// It is read and hashed once per process rather than once per build.
	sharedOnce sync.Once
	shared     sharedInputs
	sharedErr  error
	// calls counts entries into Build. Serving a request must not need the
	// builder at all, and that is invisible from the outside; the counter is
	// what lets a test pin it.
	calls atomic.Int64
}

// buildTimeout bounds one compile, including any module download it makes.
const buildTimeout = 10 * time.Minute

// sharedInputs are the build inputs that are the same for every application:
// the SDK source, its generated go.mod, and the Go version pinning it. The
// backend go.mod carries the application ID in its module path, so it is added
// to the per-application fingerprint in Build.
type sharedInputs struct {
	sdkFiles    []sourceFile
	sdkModule   string
	fingerprint string
}

func (b *Builder) sharedInputs() (sharedInputs, error) {
	b.sharedOnce.Do(func() {
		sdk, err := collect(applications.Source())
		if err != nil {
			b.sharedErr = fmt.Errorf("read backend sdk: %w", err)
			return
		}
		sdkModule := b.sdkModuleFile()
		b.shared = sharedInputs{
			sdkFiles:    sdk,
			sdkModule:   sdkModule,
			fingerprint: sharedFingerprintOf(sdk, sdkModule, b.pins.goVersion),
		}
	})
	return b.shared, b.sharedErr
}

// NewBuilder returns a builder that keeps its cache, generated modules, and
// build cache under root.
func NewBuilder(root, goTool string) *Builder {
	return &Builder{
		root:   root,
		goTool: goTool,
		pins:   readModulePins(),
		locks:  newKeyedLocks(),
	}
}

// binaryDir and buildDir hold compiled backends and the generated modules they
// were compiled from.
func (b *Builder) binaryDir() string { return filepath.Join(b.root, "bin") }
func (b *Builder) buildDir() string  { return filepath.Join(b.root, "build") }

// Build returns the path to a current binary for an application, compiling it if the
// cache does not already hold one. Concurrent calls for the same application build
// once; calls for different applications build in parallel.
func (b *Builder) Build(ctx context.Context, applicationID string, source fs.FS) (string, error) {
	b.calls.Add(1)
	shared, err := b.sharedInputs()
	if err != nil {
		return "", err
	}
	// Only the application's host backend tree is read here; backend/container
	// was already excluded by the catalog. The SDK behind it is the same tree
	// for every application and was hashed once.
	files, err := collect(source)
	if err != nil {
		return "", fmt.Errorf("read backend source: %w", err)
	}

	backendModule := b.backendModuleFile(applicationID)
	fingerprint := fingerprintWith(files, backendModule, shared.fingerprint)
	entry := backendEntry(source)
	binary := filepath.Join(b.binaryDir(), fmt.Sprintf("%s-%s", applicationID, fingerprint))
	plan := buildPlan{
		applicationID: applicationID,
		fingerprint:   fingerprint,
		binary:        binary,
		backendFiles:  files,
		sdkFiles:      shared.sdkFiles,
		backendModule: backendModule,
		sdkModule:     shared.sdkModule,
		entry:         entry,
	}

	unlock := b.locks.lock(applicationID)
	defer unlock()

	// Re-check inside the lock: the backend that waited here may have been
	// waiting for exactly this binary.
	if info, err := os.Stat(binary); err == nil && !info.IsDir() {
		return binary, nil
	}
	if err := b.compile(ctx, plan); err != nil {
		return "", err
	}
	b.pruneStale(applicationID, filepath.Base(binary))
	return binary, nil
}

// backendEntry selects the canonical root executable when root Go source is
// present. api/ remains a compatibility entry point for older uploaded apps.
func backendEntry(source fs.FS) string {
	entries, err := fs.ReadDir(source, ".")
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") &&
				!strings.HasSuffix(entry.Name(), "_test.go") {
				return "."
			}
		}
	}
	if info, err := fs.Stat(source, "api"); err == nil && info.IsDir() {
		return "./api"
	}
	return "."
}

// buildPlan is the immutable set of materialized inputs for one compile. It
// keeps the cache identity and the source/module bytes together while the
// builder moves them through the filesystem and toolchain boundary.
type buildPlan struct {
	applicationID string
	fingerprint   string
	binary        string
	backendFiles  []sourceFile
	sdkFiles      []sourceFile
	backendModule string
	sdkModule     string
	entry         string
}

// compile materializes a self-contained module and runs the Go toolchain over
// it. The build directory survives a failure so the generated source can be
// inspected, and is removed once it has produced a binary.
func (b *Builder) compile(ctx context.Context, plan buildPlan) error {
	goTool, err := findGoTool(b.goTool)
	if err != nil {
		return err
	}
	// The application's lock is held for the whole compile, so an invocation that
	// never returns — a module fetch against a proxy that accepts the
	// connection and then goes quiet, say — would not just hang this launch but
	// every later launch of the same application behind it. Bounding the toolchain
	// bounds the lock.
	ctx, cancel := context.WithTimeout(ctx, buildTimeout)
	defer cancel()
	work := filepath.Join(b.buildDir(), fmt.Sprintf("%s-%s", plan.applicationID, plan.fingerprint))
	if err := os.RemoveAll(work); err != nil {
		return fmt.Errorf("clear build directory: %w", err)
	}
	source := filepath.Join(work, "src")
	sdkRoot := filepath.Join(work, "sdk")

	if err := writeAll(source, plan.backendFiles); err != nil {
		return err
	}
	if err := writeAll(filepath.Join(sdkRoot, applications.PackageDir), plan.sdkFiles); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(source, "go.mod"), []byte(plan.backendModule)); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(sdkRoot, "go.mod"), []byte(plan.sdkModule)); err != nil {
		return err
	}
	if err := os.MkdirAll(b.binaryDir(), 0o755); err != nil {
		return fmt.Errorf("create binary directory: %w", err)
	}

	// -trimpath keeps the fingerprint honest: without it the binary would
	// embed the build directory's name, which contains the fingerprint.
	//
	// -buildvcs=false because this tree is generated, never checked out: there
	// is no revision to stamp. Left on, the toolchain probes for a repository
	// anyway and fails the whole build on a host where that probe errors
	// instead of reporting "no repository" — which would make every backend
	// uninstallable for a reason that has nothing to do with the backend.
	arguments := []string{"build", "-trimpath", "-buildvcs=false", "-o", plan.binary + ".tmp", plan.entry}
	offlineOutput, offlineErr := runGo(ctx, goTool, source, goEnv(b.root, true), arguments)
	if offlineErr != nil {
		// Falling back to the network covers the case the offline path cannot:
		// a backend that needs a module the server itself does not link, and a
		// module cache that has been pruned.
		output, err := runGo(ctx, goTool, source, goEnv(b.root, false), arguments)
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return fmt.Errorf(
					"compile backend %q: gave up after %s\n%s",
					plan.applicationID, buildTimeout, strings.TrimSpace(output))
			}
			return fmt.Errorf(
				"compile backend %q:\n%s\n(offline attempt: %s)",
				plan.applicationID, strings.TrimSpace(output), strings.TrimSpace(offlineOutput))
		}
	}
	if err := os.Rename(plan.binary+".tmp", plan.binary); err != nil {
		return fmt.Errorf("install backend binary: %w", err)
	}
	if err := os.Chmod(plan.binary, 0o755); err != nil {
		return fmt.Errorf("mark backend binary executable: %w", err)
	}
	_ = os.RemoveAll(work)
	return nil
}

func runGo(ctx context.Context, goTool, dir string, env, arguments []string) (string, error) {
	command := exec.CommandContext(ctx, goTool, arguments...)
	command.Dir = dir
	command.Env = env
	output, err := command.CombinedOutput()
	return string(output), err
}

// pruneStale removes binaries this application left behind under other
// fingerprints, so editing a backend does not accumulate copies of it.
//
// A binary is named "<applicationID>-<fingerprint>", and an application id may itself
// contain a dash: matching on the "<applicationID>-" prefix alone would let application
// "s3" delete "s3-disk"'s current binary. Only the last segment is the
// fingerprint, so the name is split there and the head compared whole.
func (b *Builder) pruneStale(applicationID, keep string) {
	entries, err := os.ReadDir(b.binaryDir())
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == keep || !isBinaryOf(name, applicationID) {
			continue
		}
		_ = os.Remove(filepath.Join(b.binaryDir(), name))
	}
}

// isBinaryOf reports whether a file in the binary directory is a compiled copy
// of applicationID, under any fingerprint. It also matches the ".tmp" a failed
// compile can leave behind, which belongs to the same application and is equally
// stale.
func isBinaryOf(name, applicationID string) bool {
	dash := strings.LastIndex(strings.TrimSuffix(name, ".tmp"), "-")
	if dash < 0 {
		return false
	}
	return name[:dash] == applicationID
}
