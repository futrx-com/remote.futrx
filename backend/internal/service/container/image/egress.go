package image

// Pre-flight for the one network failure that is invisible until it is
// expensive. When a container cannot reach IPv4, the build still gets a long
// way: Docker and most host firewalls leave ip6tables untouched, so a stranded
// container reaches everything that publishes an AAAA record — apt, NodeSource,
// the npm registry and Google's CDN all succeed, and the build only dies on the
// first IPv4-only host it needs, four stages and several minutes in, reported
// as a bare connection timeout with no hint at the cause.
//
// Probing before any stage runs turns that into an immediate, actionable
// error. The probe is deliberately patient: a container that has just been
// launched has no IPv4 route until DHCP completes on the bridge, and until then
// connect() fails instantly with ENETUNREACH. That is a booting container, not
// a broken host, so waitForIPv4Egress polls rather than deciding on one look.

// ipv4EgressProbe opens a TCP connection to well-known IPv4-only resolver
// endpoints. Bash's /dev/tcp keeps this dependency-free, which matters because
// the probe runs before the install script has added anything to the rootfs.
const ipv4EgressProbe = `for endpoint in 1.1.1.1 9.9.9.9 8.8.8.8; do
    timeout 8 bash -c "exec 3<>/dev/tcp/${endpoint}/443" 2>/dev/null && exit 0
done
exit 1`

// ipv4EgressHint explains the failure in the terms an operator can act on,
// rather than the terms the probe failed in. It takes the time waited and the
// number of attempts, so the message cannot be mistaken for a single unlucky
// probe. It deliberately does not assert a cause: every candidate below has
// been the real one at least once, and naming only the most familiar of them
// sends operators to audit a firewall that may not even be installed.
const ipv4EgressHint = `the builder container had no IPv4 egress after %s (%d attempts).

The container reached the point of having a route, or never got one at all.
Check which, on the host:

  lxc launch ubuntu:24.04 nettest
  lxc exec nettest -- ip -4 addr show scope global
  lxc exec nettest -- bash -c 'timeout 5 bash -c "exec 3<>/dev/tcp/1.1.1.1/443" && echo OK'
  lxc delete -f nettest

No address on eth0 means DHCP on the bridge is not answering. An address but
no connection means forwarded IPv4 is not getting out, and the usual causes
are, in the order worth checking:

  sysctl net.ipv4.ip_forward                 must be 1 (IPv6 forwarding can be
                                             on while this is off, which is why
                                             IPv6 destinations keep working)
  lxc network get lxdbr0 ipv4.nat            must be true
  nft list ruleset                           expect a masquerade rule for the
                                             bridge subnet
  iptables -S FORWARD                        and iptables-legacy -S FORWARD:
                                             both backends are live, and a DROP
                                             in either one is enough

Docker is a common source of that last one — it sets the FORWARD policy to DROP
and allows only its own bridges. If it is installed:
  iptables -I DOCKER-USER -i lxdbr0 -j ACCEPT
  iptables -I DOCKER-USER -o lxdbr0 -j ACCEPT

Re-running infra/install.sh applies those rules for you.`
