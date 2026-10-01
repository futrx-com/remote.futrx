// Package projectstorage manages persistent data separately from container roots.
package projectstorage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/service/project"
)

type Command func(context.Context, string, ...string) (string, error)
type Manager struct {
	root, dataset, size string
	required            bool
	run                 Command
	mu                  sync.Mutex
}

func New(root, dataset, size string, required bool) *Manager {
	if size == "" {
		size = "20GiB"
	}
	return &Manager{root: filepath.Clean(root), dataset: dataset, size: size, required: required, run: func(ctx context.Context, name string, args ...string) (string, error) {
		b, e := exec.CommandContext(ctx, name, args...).CombinedOutput()
		return string(b), e
	}}
}

var datasetPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]*(/[A-Za-z0-9][A-Za-z0-9_.:-]*)+$`)
var sizePattern = regexp.MustCompile(`^([1-9][0-9]*)(MiB|GiB|TiB)$`)

func bytesForSize(size string) (uint64, error) {
	match := sizePattern.FindStringSubmatch(size)
	if match == nil {
		return 0, errors.New("invalid persistent quota: use a positive MiB, GiB or TiB value")
	}
	n, e := strconv.ParseUint(match[1], 10, 64)
	if e != nil {
		return 0, e
	}
	shift := uint(20)
	if match[2] == "GiB" {
		shift = 30
	}
	if match[2] == "TiB" {
		shift = 40
	}
	if n > ^uint64(0)>>shift {
		return 0, errors.New("persistent quota overflows")
	}
	return n << shift, nil
}
func (m *Manager) target(cwd string) (string, string, error) {
	if !datasetPattern.MatchString(m.dataset) {
		return "", "", errors.New("PROJECT_STORAGE_DATASET must name a child ZFS dataset")
	}
	parent := filepath.Dir(filepath.Clean(cwd))
	slug := filepath.Base(parent)
	if !filepath.IsAbs(cwd) || filepath.Base(cwd) != "workspace" || filepath.Dir(parent) != m.root || !project.ValidSlug(slug) {
		return "", "", errors.New("invalid managed project path")
	}
	resolved, e := filepath.EvalSymlinks(m.root)
	if e != nil || resolved != m.root {
		return "", "", errors.New("persistent storage root is unavailable or redirected")
	}
	if info, e := os.Lstat(parent); e == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", "", errors.New("project directory is redirected")
		}
	} else if !os.IsNotExist(e) {
		return "", "", e
	}
	return parent, m.dataset + "/" + slug, nil
}

type dataset struct {
	name, mount, mounted string
	quota                uint64
}

func (m *Manager) datasets(ctx context.Context) (map[string]dataset, error) {
	out, e := m.run(ctx, "zfs", "list", "-H", "-p", "-r", "-d", "1", "-t", "filesystem", "-o", "name,mountpoint,mounted,quota", m.dataset)
	if e != nil {
		return nil, fmt.Errorf("inspect persistent ZFS datasets: %w", e)
	}
	rows := map[string]dataset{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			return nil, errors.New("invalid ZFS dataset response")
		}
		q, e := strconv.ParseUint(fields[3], 10, 64)
		if e != nil {
			return nil, errors.New("invalid ZFS quota response")
		}
		rows[fields[0]] = dataset{fields[0], fields[1], fields[2], q}
	}
	parent, ok := rows[m.dataset]
	if !ok || parent.mount != m.root || parent.mounted != "yes" {
		return nil, errors.New("persistent dataset must be mounted exactly at the project storage root")
	}
	// Verify the visible mount, not merely the dataset's configured mountpoint.
	source, e := m.run(ctx, "findmnt", "-n", "-o", "SOURCE", "-M", m.root)
	if e != nil || strings.TrimSpace(source) != m.dataset {
		return nil, errors.New("project storage root is not mounted from the configured dataset")
	}
	return rows, nil
}
func (m *Manager) Ensure(ctx context.Context, meta project.Meta) error {
	cwd := meta.Cwd
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dataset == "" {
		if m.required {
			return errors.New("persistent quotas required: configure PROJECT_STORAGE_DATASET")
		}
		return nil
	}
	if !project.ValidID(meta.ID) {
		return errors.New("invalid project identity")
	}
	limit, e := bytesForSize(m.size)
	if e != nil {
		return e
	}
	path, name, e := m.target(cwd)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	rows, e := m.datasets(ctx)
	if e != nil {
		return e
	}
	current, exists := rows[name]
	if !exists {
		if _, e := os.Lstat(path); !os.IsNotExist(e) {
			return errors.New("existing project data requires an offline dataset migration; refusing to mount over it")
		}
		if _, e = m.run(ctx, "zfs", "create", "-o", "quota="+strconv.FormatUint(limit, 10), "-o", "compression=lz4", "-o", "overlay=off", "-o", "remote:project-id="+string(meta.ID), "-o", "mountpoint="+path, name); e != nil {
			return fmt.Errorf("create bounded project dataset: %w", e)
		}
	} else {
		owner, err := m.run(ctx, "zfs", "get", "-H", "-o", "value", "remote:project-id", name)
		if err != nil || strings.TrimSpace(owner) != string(meta.ID) {
			return errors.New("persistent dataset belongs to another project or requires explicit migration ownership")
		}
		if current.mount != path || current.mounted != "yes" {
			return errors.New("project dataset has an unexpected mountpoint or is not mounted")
		}
		if current.quota == 0 {
			if _, e = m.run(ctx, "zfs", "set", "quota="+strconv.FormatUint(limit, 10), name); e != nil {
				return fmt.Errorf("apply persistent project quota: %w", e)
			}
		}
	}
	// Creation/set success alone is not evidence that data will be constrained.
	info := m.inspect(ctx, cwd)
	if !info.Enforced {
		return fmt.Errorf("verify persistent project quota: %s", info.Detail)
	}
	return nil
}
func (m *Manager) Inspect(ctx context.Context, cwd string) project.PersistentQuota {
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return m.inspect(ctx, cwd)
}
func (m *Manager) inspect(ctx context.Context, cwd string) project.PersistentQuota {
	info := project.PersistentQuota{Required: m.required || m.dataset != ""}
	if m.dataset == "" {
		info.Detail = "Persistent data is not quota-managed; configure a project storage dataset."
		return info
	}
	info.Driver = "zfs"
	path, name, e := m.target(cwd)
	if e != nil {
		info.Detail = e.Error()
		return info
	}
	rows, e := m.datasets(ctx)
	if e != nil {
		info.Detail = "Persistent quota state is unavailable."
		return info
	}
	current, ok := rows[name]
	if !ok || current.mount != path || current.mounted != "yes" || current.quota == 0 {
		info.Detail = "Project dataset is missing, unmounted or unlimited."
		return info
	}
	source, e := m.run(ctx, "findmnt", "-n", "-o", "SOURCE", "-M", path)
	if e != nil || strings.TrimSpace(source) != name {
		info.Detail = "Project path is not mounted from its quota-managed dataset."
		return info
	}
	info.Enforced = true
	info.LimitBytes = &current.quota
	return info
}
