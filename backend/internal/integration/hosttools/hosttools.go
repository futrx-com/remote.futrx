// Package hosttools installs the host-side executables an installable image
// asks for, and tells the rest of the server where they ended up.
//
// Remote itself needs none of them. Nothing here carries a package name, a
// version or a download URL: an image declares its own tool, and this package
// fetches exactly what that declaration says over HTTPS and refuses anything
// whose SHA-256 does not match. A host that installs no such image keeps
// precisely the software it was provisioned with, which is what makes an image
// that needs a host binary a genuinely optional addition rather than a
// dependency every Remote operator inherits.
//
// Tools are installed under the server's data directory, never into /usr, and
// never through the host package manager: nothing outside Remote's own state is
// modified, and uninstalling Remote takes them with it.
package hosttools

import (
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// maxDownload bounds an artifact before it is hashed, so a wrong or hostile URL
// cannot fill the host's disk while we are still deciding whether to trust it.
const maxDownload = 256 << 20

// Dir is where installed tools live for a given server data directory.
func Dir(dataDir string) string { return filepath.Join(dataDir, "host-tools") }

// Lookup resolves an installed tool's executable path. Tools installed from an
// image win over anything on PATH: the image pinned a checksum, an operator's
// distribution package did not, and silently preferring the unpinned one would
// make the pin decorative. A tool the host already provides is still accepted,
// so an operator who installed it themselves is not forced into a download.
func Lookup(dataDir, name string) (string, error) {
	if err := validName(name); err != nil {
		return "", err
	}
	link := filepath.Join(Dir(dataDir), "bin", name)
	if info, err := os.Stat(link); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
		return link, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is not installed on the Remote host", name)
	}
	return path, nil
}

// Installer installs image-declared tools. The zero value is not usable; call
// New.
type Installer struct {
	dataDir string
	// installs serializes work per tool. Two projects installing the same image
	// at once would otherwise race on the same target path — but a tool is a
	// download that can take minutes, and two unrelated tools write to two
	// unrelated paths, so a lock per tool is what keeps one slow download from
	// holding up every other install on the server.
	mu       sync.Mutex
	installs map[string]*sync.Mutex
	// fetch and verify are replaced in tests. Nothing else varies.
	fetch  func(context.Context, string) (io.ReadCloser, error)
	verify func(context.Context, string, []string) error
	arch   string
}

// New returns an Installer writing beneath dataDir.
func New(dataDir string) *Installer {
	return &Installer{
		dataDir:  dataDir,
		installs: map[string]*sync.Mutex{},
		fetch:    fetchHTTPS,
		verify:   runVersion,
		arch:     runtime.GOARCH,
	}
}

// lockTool acquires the lock for one tool and returns its release function.
func (in *Installer) lockTool(name string) func() {
	in.mu.Lock()
	entry, ok := in.installs[name]
	if !ok {
		entry = &sync.Mutex{}
		in.installs[name] = entry
	}
	in.mu.Unlock()

	entry.Lock()
	return entry.Unlock
}

// Ensure installs every tool that is not already present, and proves each one
// runs before reporting success. It is idempotent: an install already on disk
// costs one version check.
func (in *Installer) Ensure(ctx context.Context, tools []svc.HostTool) error {
	if len(tools) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	for _, tool := range tools {
		if err := in.ensureOne(ctx, tool); err != nil {
			return fmt.Errorf("host tool %s: %w", tool.Name, err)
		}
	}
	return nil
}

func (in *Installer) ensureOne(ctx context.Context, tool svc.HostTool) error {
	if err := Validate(tool); err != nil {
		return err
	}
	unlock := in.lockTool(tool.Name)
	defer unlock()
	target := filepath.Join(Dir(in.dataDir), tool.Name, tool.Version, tool.Name)
	if _, err := os.Stat(target); err != nil {
		download, ok := tool.Downloads[in.arch]
		if !ok {
			return fmt.Errorf("no download for host architecture %s", in.arch)
		}
		if err := in.install(ctx, download, target); err != nil {
			return err
		}
	}
	if err := in.verify(ctx, target, versionArgs(tool)); err != nil {
		return fmt.Errorf("installed copy does not run: %w", err)
	}
	// Publishing through bin/ last means a half-installed tool is never the one
	// Lookup hands out.
	return publish(filepath.Join(Dir(in.dataDir), "bin", tool.Name), target)
}

// install downloads, checksums and decompresses one artifact into target. The
// bytes are hashed before anything is decompressed, so a mismatched archive is
// never handed to a decompressor at all.
func (in *Installer) install(ctx context.Context, download svc.HostToolDownload, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	body, err := in.fetch(ctx, download.URL)
	if err != nil {
		return err
	}
	defer body.Close()

	staged, err := os.CreateTemp(filepath.Dir(target), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(staged.Name())
	defer staged.Close()

	digest := sha256.New()
	limited := &io.LimitedReader{R: body, N: maxDownload + 1}
	if _, err := io.Copy(io.MultiWriter(staged, digest), limited); err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if limited.N <= 0 {
		return fmt.Errorf("download exceeds %d bytes", maxDownload)
	}
	if got := hex.EncodeToString(digest.Sum(nil)); !strings.EqualFold(got, download.SHA256) {
		return fmt.Errorf("download checksum %s does not match the declared %s", got, download.SHA256)
	}
	if _, err := staged.Seek(0, io.SeekStart); err != nil {
		return err
	}

	binary, err := decompress(download.Compression, staged)
	if err != nil {
		return err
	}
	extracted, err := os.CreateTemp(filepath.Dir(target), ".extract-*")
	if err != nil {
		return err
	}
	defer os.Remove(extracted.Name())
	// One byte of headroom over the limit turns "the artifact is too big" into
	// an error instead of a silently truncated executable: io.Copy against a
	// LimitedReader that runs out stops without one, and a chopped-off binary
	// that fails to run later is the worst possible way to learn this.
	room := &io.LimitedReader{R: binary, N: maxDownload + 1}
	if _, err := io.Copy(extracted, room); err != nil {
		extracted.Close()
		return fmt.Errorf("decompress: %w", err)
	}
	if room.N <= 0 {
		extracted.Close()
		return fmt.Errorf("decompressed artifact exceeds %d bytes", maxDownload)
	}
	if err := extracted.Chmod(0o755); err != nil {
		extracted.Close()
		return err
	}
	if err := extracted.Close(); err != nil {
		return err
	}
	// Rename last: the target path either does not exist or is a complete,
	// verified executable, never a partial one another install could pick up.
	return os.Rename(extracted.Name(), target)
}

func decompress(compression string, r io.Reader) (io.Reader, error) {
	switch compression {
	case "":
		return r, nil
	case "gzip":
		return gzip.NewReader(r)
	case "bzip2":
		return bzip2.NewReader(r), nil
	default:
		return nil, fmt.Errorf("unsupported compression %q", compression)
	}
}

// publish points <dir>/bin/<name> at the versioned copy, replacing whatever it
// pointed at before.
func publish(link, target string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	staged := link + ".new"
	if err := os.Remove(staged); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(target, staged); err != nil {
		return err
	}
	return os.Rename(staged, link)
}

// Validate reports whether a declaration is installable at all. The registry
// calls it at catalog load, so a malformed tool fails when the image is loaded
// rather than on the host of the first person who installs it.
func Validate(tool svc.HostTool) error {
	if err := validName(tool.Name); err != nil {
		return err
	}
	if tool.Version == "" || strings.ContainsAny(tool.Version, `/\`) || tool.Version == "." || tool.Version == ".." {
		return fmt.Errorf("invalid version %q", tool.Version)
	}
	if len(tool.Downloads) == 0 {
		return fmt.Errorf("no downloads declared")
	}
	for arch, download := range tool.Downloads {
		if arch == "" || strings.ContainsAny(arch, `/\`) {
			return fmt.Errorf("invalid architecture %q", arch)
		}
		u, err := url.Parse(download.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fmt.Errorf("%s download must be an https URL without credentials", arch)
		}
		if raw, err := hex.DecodeString(download.SHA256); err != nil || len(raw) != sha256.Size {
			return fmt.Errorf("%s download needs a hex sha256 digest", arch)
		}
		switch download.Compression {
		case "", "gzip", "bzip2":
		default:
			return fmt.Errorf("%s download has unsupported compression %q", arch, download.Compression)
		}
	}
	for _, arg := range versionArgs(tool) {
		if strings.HasPrefix(arg, "-") && strings.ContainsAny(arg, " \t") {
			return fmt.Errorf("invalid version argument %q", arg)
		}
	}
	return nil
}

func validName(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || strings.HasPrefix(name, ".") {
		return fmt.Errorf("invalid tool name %q", name)
	}
	return nil
}

func versionArgs(tool svc.HostTool) []string {
	if len(tool.VersionArgs) > 0 {
		return tool.VersionArgs
	}
	return []string{"version"}
}

func fetchHTTPS(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		// The URL can carry no credentials (Validate rejects those), so it is
		// safe to report which download failed.
		return nil, fmt.Errorf("download %s: %w", rawURL, err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("download %s: HTTP %d", rawURL, response.StatusCode)
	}
	return response.Body, nil
}

func runVersion(ctx context.Context, path string, args []string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// Output is discarded rather than reported: a tool may echo a repository
	// URL or credential from its environment.
	return exec.CommandContext(ctx, path, args...).Run()
}
