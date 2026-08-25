package applications

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// portScanRange bounds how far past the preferred port the allocator probes
// before giving up.
const portScanRange = 512

// HostPortAllocator picks a free host TCP port for a new proxy device. It
// avoids ports already bound on the host (discovered with `ss`) and any port in
// the caller-supplied reserved set (ports persisted to other instances).
type HostPortAllocator struct{}

// NewHostPortAllocator constructs the default allocator.
func NewHostPortAllocator() *HostPortAllocator { return &HostPortAllocator{} }

var _ svc.PortAllocator = (*HostPortAllocator)(nil)

// Allocate returns the first free host port at or after preferred that is
// neither in taken nor currently listening on the host.
func (a *HostPortAllocator) Allocate(ctx context.Context, _ string, preferred int, taken map[int]bool) (int, error) {
	if preferred < 1024 || preferred > 65535 {
		preferred = 1024
	}
	bound := hostListeningPorts(ctx)
	for p := preferred; p <= preferred+portScanRange && p <= 65535; p++ {
		if taken[p] || bound[p] {
			continue
		}
		return p, nil
	}
	return 0, fmt.Errorf("no free host port near %d", preferred)
}

// hostListeningPorts returns the set of TCP ports currently bound on the host.
// Best-effort: if `ss` is unavailable the set is empty and the reserved set
// alone guards conflicts.
func hostListeningPorts(ctx context.Context) map[int]bool {
	out := map[int]bool{}
	cmd := exec.CommandContext(ctx, "ss", "-H", "-ltnu")
	raw, err := cmd.Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		// ss columns: Netid State Recv-Q Send-Q Local-Address:Port Peer... ;
		// the local address is the 5th field for -t, 4th for some layouts —
		// scan every field for a host:port token instead of guessing.
		for _, f := range fields {
			if p, ok := portFromAddr(f); ok {
				out[p] = true
			}
		}
	}
	return out
}

// portFromAddr extracts the port from a "host:port" or "*:port" token.
func portFromAddr(tok string) (int, bool) {
	i := strings.LastIndex(tok, ":")
	if i < 0 || i == len(tok)-1 {
		return 0, false
	}
	p, err := strconv.Atoi(tok[i+1:])
	if err != nil || p <= 0 || p > 65535 {
		return 0, false
	}
	return p, true
}
