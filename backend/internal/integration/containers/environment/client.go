// Package environment applies project environment-variable configuration to
// containers.
package environment

// ApplyContainerEnvDiff drives LXD's built-in environment.* config keys.
// Used by the project-secrets flow to push per-project secrets into the
// project's container, so any subsequent `lxc exec` (agent prompt or
// interactive shell) inherits the var automatically.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
)

const queryTimeout = 10 * time.Second

// Client owns container environment configuration.
type Client struct {
	runner command.Runner
}

// NewClient returns an environment client backed by runner.
func NewClient(runner command.Runner) *Client {
	return &Client{runner: runner}
}

// ApplyContainerEnvDiff sets the values in set and removes the keys in unset.
// Idempotent: a set that already matches the current value is a no-op (cheap
// read before write). Returns the first error encountered.
//
// Either map can be nil/empty. Already-running processes inside the container
// keep their fork-time environ; only new exec sessions and child processes
// pick up the new values — unavoidable env-var caveat.
func (s *Client) ApplyDiff(
	ctx context.Context,
	container string,
	set map[string]string,
	unset []string,
) error {
	if !s.runner.Available() {
		return command.ErrUnavailable
	}
	if strings.TrimSpace(container) == "" {
		return errors.New("container name required")
	}

	keysToUnset := make([]string, 0, len(unset)+len(set))
	keysToUnset = append(keysToUnset, unset...)
	for k, v := range set {
		// LXD rejects line breaks in persistent environment.* config. Agent
		// sessions still receive multiline project secrets through `lxc exec
		// --env`. Remove any previously persisted single-line value so rotating a
		// secret to PEM/JSON cannot leave stale credentials in terminal sessions.
		if strings.ContainsAny(v, "\r\n") {
			keysToUnset = append(keysToUnset, k)
			continue
		}
		cur, _ := command.RunWithTimeout(ctx, s.runner, queryTimeout, "config", "get", container, "environment."+k)
		if strings.TrimSpace(cur) == v {
			continue
		}
		// End flag parsing before the value. PEM/OpenSSH secrets commonly start
		// with "-----BEGIN", which LXC otherwise interprets as a CLI flag.
		_, err := command.RunWithTimeout(ctx, s.runner, queryTimeout, "config", "set", container, "environment."+k, "--", v)
		if err != nil {
			// LXC can echo an invalid argument in its output. Never attach that
			// output here because the argument is the secret value itself.
			return fmt.Errorf("set environment.%s on %s: %w", k, container, err)
		}
	}

	for _, k := range keysToUnset {
		out, err := command.RunWithTimeout(ctx, s.runner, queryTimeout, "config", "unset", container, "environment."+k)
		if err != nil {
			// `lxc config unset` on a missing key still exits non-zero;
			// treat that as success.
			if strings.Contains(out, "not set") || strings.Contains(out, "not currently set") || strings.Contains(out, "doesn't exist") {
				continue
			}
			return fmt.Errorf("unset environment.%s on %s: %w; output: %s", k, container, err, out)
		}
	}
	return nil
}
