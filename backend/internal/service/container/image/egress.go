package image

import (
	"context"
	"errors"
	"time"
)

// Pre-flight for the one network failure that is invisible until it is
// expensive. Docker sets the host's iptables FORWARD policy to DROP and
// accepts only its own bridges, which leaves LXD containers with no IPv4
// egress. Docker does not touch ip6tables, so a stranded container still
// reaches everything that publishes an AAAA record: apt, NodeSource, the npm
// registry and Google's CDN all succeed, and the build only dies on the first
// IPv4-only host it needs — github.com, four stages and several minutes in,
// reported as a bare connection timeout with no hint at the cause.
//
// Probing once, before any stage runs, turns that into an immediate and
// actionable error. See infra/lib/container-forwarding.sh for the fix the
// installer applies.

// ipv4EgressProbe opens a TCP connection to well-known IPv4-only resolver
// endpoints. Bash's /dev/tcp keeps this dependency-free, which matters because
// the probe runs before the install script has added anything to the rootfs.
const ipv4EgressProbe = `for endpoint in 1.1.1.1 9.9.9.9 8.8.8.8; do
    timeout 8 bash -c "exec 3<>/dev/tcp/${endpoint}/443" 2>/dev/null && exit 0
done
exit 1`

// ipv4EgressHint explains the failure in the terms an operator can act on,
// rather than the terms the probe failed in.
const ipv4EgressHint = `the builder container cannot reach any IPv4 address.

This is usually Docker: it sets the host's iptables FORWARD policy to DROP and
allows only its own bridges, so LXD containers lose IPv4 while IPv6 keeps
working — which is why apt and npm succeed and only IPv4-only hosts fail.

Fix it by re-running infra/install.sh, which configures this automatically, or
by hand:
  iptables -I DOCKER-USER -i lxdbr0 -j ACCEPT
  iptables -I DOCKER-USER -o lxdbr0 -j ACCEPT`

// Vars rather than consts so a test can shorten them; nothing else writes.
var (
	ipv4EgressDeadline = 45 * time.Second
	ipv4EgressInterval = 3 * time.Second
)

// egressProber is the slice of the runtime awaitIPv4Egress needs.
type egressProber interface {
	ExecuteScript(ctx context.Context, container, script string) (string, error)
}

// awaitIPv4Egress waits for the builder to reach an IPv4 address, and reports
// the hint above only once it is clear that waiting will not help.
//
// The probe used to run once, after a fixed warmup. On a slow host the
// builder's DHCP lease can take longer than that warmup, so the probe ran
// against a container with no address yet and blamed Docker's FORWARD policy —
// which sends an operator to inspect iptables on a machine that has no Docker
// installed. Retrying separates the two cases that single shot conflated: a
// container still coming up gets the time it needs, and one with no IPv4 route
// still fails with the same actionable message, just later.
//
// A cancelled build returns the context error unchanged, so an interrupted run
// is never reported as a network fault.
func awaitIPv4Egress(ctx context.Context, runtime egressProber, container string) error {
	deadline := time.Now().Add(ipv4EgressDeadline)
	for {
		if _, err := runtime.ExecuteScript(ctx, container, ipv4EgressProbe); err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if time.Now().After(deadline) {
			return errors.New(ipv4EgressHint)
		}
		select {
		case <-time.After(ipv4EgressInterval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
