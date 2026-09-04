package pluginhost

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/futrx-com/remote.futrx.com/pkg/appplugin"
)

// Builder turns an image's plugin/ source into an executable, caching the
// result by a fingerprint of everything that went into it.
//
// The catalog ships source rather than binaries because it is embedded in the
// server and has to stay portable across the architectures a server runs on.
// Compiling once per image build and caching by fingerprint keeps the cost off
// every install: a cold build takes seconds, a warm one is a stat.
type Builder struct {
	root   string
	goTool string
	pins   modulePins
	locks  keyedLocks
	// calls counts entries into Build. Serving a request must not need the
	// builder at all, and that is invisible from the outside; the counter is
	// what lets a test pin it.
	calls atomic.Int64
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

// binaryDir and buildDir hold compiled plugins and the generated modules they
// were compiled from.
func (b *Builder) binaryDir() string { return filepath.Join(b.root, "bin") }
func (b *Builder) buildDir() string  { return filepath.Join(b.root, "build") }

// Build returns the path to a current binary for an image, compiling it if the
// cache does not already hold one. Concurrent calls for the same image build
// once; calls for different images build in parallel.
func (b *Builder) Build(ctx context.Context, imageID string, source fs.FS) (string, error) {
	b.calls.Add(1)
	files, err := collect(source)
	if err != nil {
		return "", fmt.Errorf("read plugin source: %w", err)
	}
	sdk, err := collect(appplugin.Source())
	if err != nil {
		return "", fmt.Errorf("read plugin sdk: %w", err)
	}
	pluginModule := b.pluginModuleFile()
	sdkModule := b.sdkModuleFile()

	fingerprint := fingerprintOf(files, sdk, pluginModule, sdkModule, b.pins.goVersion)
	binary := filepath.Join(b.binaryDir(), fmt.Sprintf("%s-%s", imageID, fingerprint))
	plan := buildPlan{
		imageID:      imageID,
		fingerprint:  fingerprint,
		binary:       binary,
		pluginFiles:  files,
		sdkFiles:     sdk,
		pluginModule: pluginModule,
		sdkModule:    sdkModule,
	}

	unlock := b.locks.lock(imageID)
	defer unlock()

	// Re-check inside the lock: the plugin that waited here may have been
	// waiting for exactly this binary.
	if info, err := os.Stat(binary); err == nil && !info.IsDir() {
		return binary, nil
	}
	if err := b.compile(ctx, plan); err != nil {
		return "", err
	}
	b.pruneStale(imageID, filepath.Base(binary))
	return binary, nil
}

// buildPlan is the immutable set of materialized inputs for one compile. It
// keeps the cache identity and the source/module bytes together while the
// builder moves them through the filesystem and toolchain boundary.
type buildPlan struct {
	imageID      string
	fingerprint  string
	binary       string
	pluginFiles  []sourceFile
	sdkFiles     []sourceFile
	pluginModule string
	sdkModule    string
}

// compile materializes a self-contained module and runs the Go toolchain over
// it. The build directory survives a failure so the generated source can be
// inspected, and is removed once it has produced a binary.
func (b *Builder) compile(ctx context.Context, plan buildPlan) error {
	goTool, err := findGoTool(b.goTool)
	if err != nil {
		return err
	}
	work := filepath.Join(b.buildDir(), fmt.Sprintf("%s-%s", plan.imageID, plan.fingerprint))
	if err := os.RemoveAll(work); err != nil {
		return fmt.Errorf("clear build directory: %w", err)
	}
	source := filepath.Join(work, "src")
	sdkRoot := filepath.Join(work, "sdk")

	if err := writeAll(source, plan.pluginFiles); err != nil {
		return err
	}
	if err := writeAll(filepath.Join(sdkRoot, appplugin.PackageDir), plan.sdkFiles); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(source, "go.mod"), []byte(plan.pluginModule)); err != nil {
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
	arguments := []string{"build", "-trimpath", "-o", plan.binary + ".tmp", "."}
	offlineOutput, offlineErr := runGo(ctx, goTool, source, goEnv(b.root, true), arguments)
	if offlineErr != nil {
		// Falling back to the network covers the case the offline path cannot:
		// a plugin that needs a module the server itself does not link, and a
		// module cache that has been pruned.
		output, err := runGo(ctx, goTool, source, goEnv(b.root, false), arguments)
		if err != nil {
			return fmt.Errorf(
				"compile plugin %q:\n%s\n(offline attempt: %s)",
				plan.imageID, strings.TrimSpace(output), strings.TrimSpace(offlineOutput))
		}
	}
	if err := os.Rename(plan.binary+".tmp", plan.binary); err != nil {
		return fmt.Errorf("install plugin binary: %w", err)
	}
	if err := os.Chmod(plan.binary, 0o755); err != nil {
		return fmt.Errorf("mark plugin binary executable: %w", err)
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

// pruneStale removes binaries this image left behind under other
// fingerprints, so editing a plugin does not accumulate copies of it.
func (b *Builder) pruneStale(imageID, keep string) {
	entries, err := os.ReadDir(b.binaryDir())
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == keep || !strings.HasPrefix(name, imageID+"-") {
			continue
		}
		_ = os.Remove(filepath.Join(b.binaryDir(), name))
	}
}
