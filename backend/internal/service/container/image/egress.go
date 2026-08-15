package image

import "strings"

// Pre-flight for the one network failure that is invisible until it is
// expensive. A container that can reach IPv6 but not IPv4 still reaches
// everything that publishes an AAAA record — apt, NodeSource, the npm registry
// and Google's CDN all succeed — and the build only dies on the first
// IPv4-only host it needs: github.com, four stages and several minutes in,
// reported as a bare connection timeout with no hint at the cause.
//
// Probing once, before any stage runs, turns that into an immediate error.
//
// The harder problem is saying something true about *why*. At least five
// distinct host conditions produce an identical symptom, and an earlier
// version of this file asserted the Docker one unconditionally. On a host
// without Docker that produced a confident, wrong instruction — `iptables -I
// DOCKER-USER ...`, which fails with "No chain/target/match by that name" —
// and no path to the real cause.
//
// So the probe now reports which hop failed, and the hint branches on that.
// The three hops are genuinely different problems with disjoint fixes:
//
//	no address  → DHCP never answered; the bridge's dnsmasq is not running,
//	              usually because something else (BIND, on a Plesk box) holds
//	              port 53 on the bridge address. No firewall change helps.
//	no route    → the lease came without a gateway; the bridge is misconfigured.
//	blocked     → address and route are fine and packets are being dropped or
//	              not masqueraded. This is the forwarding case Docker causes.
//
// See infra/lib/container-forwarding.sh and infra/lib/bridge-dns.sh for the
// fixes the installer applies, and infra/diagnose-network.sh for the
// host-side version of this same triage.

// diagnosePath is where the installer puts the host-side diagnostic. Every
// branch ends by pointing at it, because the container can only see which hop
// failed — naming the process holding the port, or the rule doing the
// dropping, needs a look at the host.
const diagnosePath = "/opt/remote.futrx/infra/diagnose-network.sh"

// Markers the probe prints so the hint can branch on evidence rather than
// assumption. They are matched as substrings of the probe's output.
const (
	egressNoAddress = "futrx-egress: no-ipv4-address"
	egressNoRoute   = "futrx-egress: no-default-route"
	egressBlocked   = "futrx-egress: blocked"
)

// ipv4EgressProbe checks the three hops in order and names the first one that
// fails. Bash's /dev/tcp keeps the reachability test dependency-free, which
// matters because this runs before the install script has added anything to
// the rootfs.
const ipv4EgressProbe = `if ! ip -4 -o addr show scope global 2>/dev/null | grep -q .; then
    echo "futrx-egress: no-ipv4-address"
    exit 1
fi
if ! ip -4 route show default 2>/dev/null | grep -q .; then
    echo "futrx-egress: no-default-route"
    exit 1
fi
for endpoint in 1.1.1.1 9.9.9.9 8.8.8.8; do
    timeout 8 bash -c "exec 3<>/dev/tcp/${endpoint}/443" 2>/dev/null && exit 0
done
echo "futrx-egress: blocked"
exit 1`

// ipv4EgressHint explains the failure in the terms an operator can act on,
// rather than the terms the probe failed in. probeOutput is whatever the
// probe printed; an unrecognised value falls back to the generic message
// rather than guessing at a cause.
func ipv4EgressHint(probeOutput string) string {
	switch {
	case strings.Contains(probeOutput, egressNoAddress):
		return `the builder container never received an IPv4 address.

This is a DHCP failure, not a firewall one: LXD runs a dnsmasq bound to the
lxdbr0 address to serve leases, and if something else already holds port 53
there it never starts. The container boots with IPv6 link-local only, which is
why apt and npm still work and only IPv4-only hosts fail.

On a Plesk server the usual culprit is BIND — its default listen-on { any; }
binds every interface address it finds, including the bridge.

  sudo ss -lnup | grep :53
  sudo bash ` + diagnosePath + `

Re-running infra/install.sh fixes this automatically when Plesk is present.`

	case strings.Contains(probeOutput, egressNoRoute):
		return `the builder container has an IPv4 address but no default route.

Its DHCP lease arrived without a gateway, which means the lxdbr0 bridge is
configured without one. Check the bridge:

  lxc network get lxdbr0 ipv4.address    # must be a subnet, not "none"
  lxc network get lxdbr0 ipv4.nat        # must be true
  sudo bash ` + diagnosePath

	case strings.Contains(probeOutput, egressBlocked):
		return `the builder container has IPv4 but cannot reach the internet.

Its packets are being dropped on the way out or leaving without being
masqueraded. Common causes, in the order worth checking:

  • Docker, which sets the host's iptables FORWARD policy to DROP and accepts
    only its own bridges. It leaves ip6tables alone, which is why IPv6 keeps
    working.
  • Any other host firewall doing the same with a rule rather than a policy —
    Plesk's firewall module reconciles its ruleset on its own schedule.
  • net.ipv4.ip_forward = 0. IPv6 forwarding is a separate knob.
  • Rules applied to one iptables backend while the DROP lives in the other.

  sudo bash ` + diagnosePath + `

Re-running infra/install.sh applies and then keeps re-applying the fix.`
	}

	return `the builder container cannot reach any IPv4 address.

The probe could not determine which hop failed. Run the host-side diagnostic,
which checks forwarding, both iptables backends, native nftables, the bridge's
NAT and DHCP, and a throwaway container's own connectivity:

  sudo bash ` + diagnosePath
}
