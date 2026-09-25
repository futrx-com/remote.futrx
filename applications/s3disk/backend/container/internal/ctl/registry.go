package ctl

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Entry records a live mount so the CLI can find its control socket.
type Entry struct {
	Mountpoint string    `json:"mountpoint"`
	Bucket     string    `json:"bucket"`
	Prefix     string    `json:"prefix"`
	CacheDir   string    `json:"cache_dir"`
	Socket     string    `json:"socket"`
	PID        int       `json:"pid"`
	Started    time.Time `json:"started"`
}

// RegistryDir is where live-mount records are kept.
func RegistryDir() string {
	if d := os.Getenv("S3DISK_RUN_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "s3disk")
	}
	if err := os.MkdirAll("/run/s3disk", 0700); err == nil {
		return "/run/s3disk"
	}
	return filepath.Join(os.TempDir(), "s3disk")
}

func registryFile(mountpoint string) string {
	abs, err := filepath.Abs(mountpoint)
	if err != nil {
		abs = mountpoint
	}
	name := strings.ReplaceAll(strings.TrimPrefix(abs, "/"), "/", "-")
	if name == "" {
		name = "root"
	}
	return filepath.Join(RegistryDir(), name+".json")
}

// Register writes the record for a mount that is coming up.
func Register(e Entry) error {
	if err := os.MkdirAll(RegistryDir(), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(registryFile(e.Mountpoint), data, 0600)
}

// Deregister removes the record for a mount that has gone away.
func Deregister(mountpoint string) error {
	err := os.Remove(registryFile(mountpoint))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// List returns every live mount, pruning records whose socket is dead.
func List() []Entry {
	dir := RegistryDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Entry
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			continue
		}
		var e Entry
		if json.Unmarshal(data, &e) != nil {
			continue
		}
		if !alive(e.Socket) {
			_ = os.Remove(filepath.Join(dir, f.Name()))
			continue
		}
		out = append(out, e)
	}
	return out
}

// Find locates the mount serving a path, or the single mount if path is empty.
func Find(path string) (Entry, bool) {
	entries := List()
	if path == "" {
		if len(entries) == 1 {
			return entries[0], true
		}
		return Entry{}, false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for _, e := range entries {
		if e.Mountpoint == abs {
			return e, true
		}
	}
	// Accept any path inside a mount, so `s3disk status .` works.
	for _, e := range entries {
		if strings.HasPrefix(abs, e.Mountpoint+"/") {
			return e, true
		}
	}
	return Entry{}, false
}

func alive(socket string) bool {
	if socket == "" {
		return false
	}
	c, err := net.DialTimeout("unix", socket, 300*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}
