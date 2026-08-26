package gitcli

import (
	"context"
	"errors"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

const cloneTimeout = 10 * time.Minute

// ErrCloneFailed is returned for any clone failure. The underlying git output
// is logged server-side only; it is never safe to surface verbatim to a
// client since git's own error text for a private repo is indistinguishable
// from a typo or an unreachable host.
var ErrCloneFailed = errors.New(
	"couldn't clone the repository — check the URL is public and reachable; " +
		"to import a private repo, create the project first and clone it via the Terminal",
)

// Cloner seeds a new, host-side project workspace directory by cloning a
// public git repository into it.
type Cloner struct{}

func NewCloner() *Cloner {
	return &Cloner{}
}

// Clone clones url into dest. dest must already exist (created by the
// caller's workspace preparation step). If dest already contains entries,
// Clone assumes the workspace was already seeded by an earlier provisioning
// pass (e.g. a container recreated for an existing project) and does
// nothing — this makes Clone safe to call on every provisioning pass rather
// than only on first-ever creation.
func (c *Cloner) Clone(ctx context.Context, url, dest string) error {
	entries, err := os.ReadDir(dest)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return nil
	}

	cloneCtx, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()

	cmd := exec.CommandContext(cloneCtx, "git", "clone", "--", url, dest)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("gitcli: clone %s failed: %v: %s", url, err, strings.TrimSpace(string(output)))
		return ErrCloneFailed
	}
	return nil
}
