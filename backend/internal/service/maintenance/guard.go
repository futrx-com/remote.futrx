// Package maintenance exposes the file-backed maintenance window used by
// host infrastructure jobs. The marker includes its owner's PID so an
// interrupted updater cannot leave Remote permanently locked.
package maintenance

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const maxMarkerAge = 2 * time.Hour

type marker struct {
	PID       int   `json:"pid"`
	StartedAt int64 `json:"startedAt"`
}

type Guard struct {
	path  string
	now   func() time.Time
	alive func(int) bool
}

func New(dataDir string) *Guard {
	return &Guard{
		path: filepath.Join(dataDir, "self-update", "maintenance.json"),
		now:  time.Now,
		alive: func(pid int) bool {
			if pid <= 0 {
				return false
			}
			err := syscall.Kill(pid, 0)
			return err == nil || errors.Is(err, syscall.EPERM)
		},
	}
}

// Blocked reports whether a live infrastructure job owns the maintenance
// window. Invalid, stale, and orphaned markers fail open so a killed updater
// cannot prevent future prompts indefinitely.
func (g *Guard) Blocked() bool {
	if g == nil {
		return false
	}
	data, err := os.ReadFile(g.path)
	if err != nil {
		return false
	}
	var current marker
	if json.Unmarshal(data, &current) != nil || current.PID <= 0 || current.StartedAt <= 0 {
		return false
	}
	age := g.now().Sub(time.Unix(current.StartedAt, 0))
	if age < 0 || age > maxMarkerAge {
		return false
	}
	return g.alive(current.PID)
}
