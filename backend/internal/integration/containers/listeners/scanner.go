// Package listeners discovers externally reachable TCP listeners inside
// project containers.
package listeners

// ListListeners surfaces every externally-reachable TCP listener inside a
// container so the UI's Browser drawer can offer them as a pick list. We
// rely on `ss -tlnHp` (iproute2, always present on the base image) and
// filter out loopback bindings — those are invisible to Caddy because the
// reverse_proxy targets <slug>.lxd:<port>, not 127.0.0.1.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

const listenersTimeout = 5 * time.Second

// Scanner discovers container applications from listening TCP sockets.
type Scanner struct {
	runner command.Runner
}

// NewScanner returns a listener scanner backed by runner.
func NewScanner(runner command.Runner) *Scanner {
	return &Scanner{runner: runner}
}

func (s *Scanner) List(ctx context.Context, containerName string) ([]serviceproject.ContainerApp, error) {
	if !s.runner.Available() {
		return nil, command.ErrUnavailable
	}
	// -t TCP, -l listening, -n numeric, -H no header, -p process info.
	// We accept the inevitable non-zero exit if ss isn't installed (the
	// base image always has iproute2, but a stripped derivative might not).
	out, err := command.RunWithTimeout(ctx, s.runner, listenersTimeout, "exec", containerName, "--", "ss", "-tlnHp")
	if err != nil {
		return nil, fmt.Errorf("ss in container: %w; output: %s", err, out)
	}
	return parseSS(out), nil
}

// parseSS walks `ss -tlnHp` output and emits one ContainerApp per
// externally-reachable port. Duplicate ports (IPv4 + IPv6 of the same
// process) collapse into one entry — we prefer the IPv4 row but if only
// IPv6 exists we keep it.
func parseSS(raw string) []serviceproject.ContainerApp {
	type slot struct {
		app    serviceproject.ContainerApp
		isIPv4 bool
	}
	byPort := map[int]slot{}

	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		// State Recv-Q Send-Q Local-Address:Port Peer-Address:Port [users:...]
		if len(fields) < 4 {
			continue
		}
		host, port, ok := SplitListenerAddr(fields[3])
		if !ok {
			continue
		}
		if isLoopback(host) {
			continue
		}

		isV4 := !strings.Contains(host, ":")
		app := serviceproject.ContainerApp{
			Port:    port,
			Address: host,
		}
		if len(fields) >= 6 {
			app.Process, app.PID = parseUsers(strings.Join(fields[5:], " "))
		}

		// Keep one entry per port, preferring IPv4 over IPv6 for nicer
		// display ("0.0.0.0:3000" vs "[::]:3000").
		if cur, exists := byPort[port]; exists {
			if cur.isIPv4 {
				continue
			}
		}
		byPort[port] = slot{app: app, isIPv4: isV4}
	}

	out := make([]serviceproject.ContainerApp, 0, len(byPort))
	for _, s := range byPort {
		out = append(out, s.app)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Port < out[j].Port })
	return out
}

// SplitListenerAddr parses an `ss` Local-Address column.
// Accepts "0.0.0.0:3000", "127.0.0.1:53", "127.0.0.53%lo:53", "[::]:22",
// "[::1]:53", "*:3000". Returns the bare host (no brackets, no %iface)
// and the integer port.
//
// It is exported because it is the one piece of `ss` output shape anything
// outside this package needs: the host port allocator reads the same column
// from the same tool, and a second parser of it would be a second thing to fix
// when a layout surprises us.
func SplitListenerAddr(addr string) (string, int, bool) {
	colon := strings.LastIndex(addr, ":")
	if colon < 0 {
		return "", 0, false
	}
	host := addr[:colon]
	portStr := addr[colon+1:]
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, false
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	if i := strings.Index(host, "%"); i >= 0 {
		host = host[:i]
	}
	if host == "*" {
		host = "0.0.0.0"
	}
	return host, port, true
}

func isLoopback(host string) bool {
	if host == "" {
		return false
	}
	if host == "::1" {
		return true
	}
	return strings.HasPrefix(host, "127.")
}

// parseUsers extracts the first process name + pid from a column like
// `users:(("node",pid=1234,fd=20),("node",pid=1235,fd=21))`.
func parseUsers(s string) (string, int) {
	q := strings.Index(s, "\"")
	if q < 0 {
		return "", 0
	}
	rest := s[q+1:]
	q2 := strings.Index(rest, "\"")
	if q2 < 0 {
		return "", 0
	}
	name := rest[:q2]
	pidIdx := strings.Index(rest, "pid=")
	if pidIdx < 0 {
		return name, 0
	}
	pidStr := rest[pidIdx+4:]
	end := strings.IndexAny(pidStr, ",) ")
	if end < 0 {
		return name, 0
	}
	pid, _ := strconv.Atoi(pidStr[:end])
	return name, pid
}
