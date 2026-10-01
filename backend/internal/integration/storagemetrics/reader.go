// Package storagemetrics bounds background scans of project-owned data.
package storagemetrics

import (
	"context"
	"errors"
	"github.com/futrx-com/remote.futrx.com/internal/service/project"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type QuotaReader interface {
	Inspect(context.Context, string) project.PersistentQuota
}

func (r *Reader) WithQuotaReader(q QuotaReader) *Reader { r.quotas = q; return r }

type Reader struct {
	quotas    QuotaReader
	root      string
	threshold float64
	mu        sync.Mutex
	entries   map[string]project.PersistentStorage
	worker    chan struct{}
}

func New(root string, threshold float64) *Reader {
	if threshold <= 0 || threshold > 100 {
		threshold = 80
	}
	return &Reader{root: filepath.Clean(root), threshold: threshold, entries: map[string]project.PersistentStorage{}, worker: make(chan struct{}, 1)}
}
func (r *Reader) Read(ctx context.Context, cwd string) (result project.PersistentStorage) {
	defer func() {
		if r.quotas != nil {
			q := r.quotas.Inspect(ctx, cwd)
			result.Quota = &q
		}
	}()
	parent := filepath.Dir(filepath.Clean(cwd))
	relative, err := filepath.Rel(r.root, parent)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") || filepath.Dir(relative) != "." || filepath.Base(cwd) != "workspace" {
		return project.PersistentStorage{Error: "workspace is outside managed project storage"}
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return project.PersistentStorage{Error: "project storage path is unavailable or redirected"}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := r.entries[parent]
	if !out.Pending && (out.SampledAt == 0 || time.Since(time.Unix(out.SampledAt, 0)) > 5*time.Minute) {
		select {
		case r.worker <- struct{}{}:
			out.Pending = true
			r.entries[parent] = out
			go func() {
				defer func() { <-r.worker }()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				result := scan(ctx, parent, 250000, r.threshold)
				r.mu.Lock()
				r.entries[parent] = result
				r.mu.Unlock()
			}()
		default:
			out.Pending = true
		}
	}
	return out
}
func scan(ctx context.Context, root string, maxEntries int, threshold float64) project.PersistentStorage {
	out := project.PersistentStorage{SampledAt: time.Now().Unix()}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err == nil && stat.Blocks > 0 {
		available := stat.Bavail * uint64(stat.Bsize)
		used := float64(stat.Blocks-stat.Bavail) * 100 / float64(stat.Blocks)
		out.AvailableBytes = &available
		out.UsagePercent = &used
		out.Warning = used >= threshold
		if stat.Files > 0 {
			inode := float64(stat.Files-stat.Ffree) * 100 / float64(stat.Files)
			out.InodePercent = &inode
			out.Warning = out.Warning || inode >= threshold
		}
	}
	var bytes uint64
	count := 0
	// WalkDir does not follow symlinks. Bound entries and time; do not expose
	// filenames or return a partial scan as a complete usage total.

	confined, err := os.OpenRoot(root)
	if err != nil {
		out.Error = "persistent storage unavailable"
		return out
	}
	defer confined.Close()
	seen := map[[2]uint64]bool{}
	var walk func(string, int) error
	walk = func(relative string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 128 {
			return errors.New("directory depth exceeded")
		}
		directory, err := confined.Open(relative)
		if err != nil {
			return err
		}
		defer directory.Close()
		for {
			entries, readErr := directory.ReadDir(128)
			for _, entry := range entries {
				if err := ctx.Err(); err != nil {
					return err
				}
				count++
				if count > maxEntries {
					return errors.New("scan budget exceeded")
				}
				if entry.Type()&os.ModeSymlink != 0 {
					continue
				}
				child := filepath.Join(relative, entry.Name())
				info, err := confined.Lstat(child)
				if err != nil {
					return err
				}
				if info.Mode()&os.ModeSymlink != 0 {
					continue
				}
				if data, ok := info.Sys().(*syscall.Stat_t); ok {
					key := [2]uint64{uint64(data.Dev), data.Ino}
					if !seen[key] {
						bytes += uint64(data.Blocks) * 512
						seen[key] = true
					}
				}
				if info.IsDir() {
					if err := walk(child, depth+1); err != nil {
						return err
					}
				}
			}
			if readErr == io.EOF {
				return nil
			}
			if readErr != nil {
				return readErr
			}
		}
	}
	err = walk(".", 0)
	if err != nil {
		out.Error = "persistent storage scan incomplete; retry later"
	} else {
		out.Bytes = &bytes
	}
	return out
}
