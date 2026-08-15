#!/usr/bin/env bash
set -euo pipefail

TEST_TMP="$(mktemp -d)"
trap 'rm -rf "$TEST_TMP"' EXIT

# Keep the BIND include inside the sandbox — this suite must never be able to
# write to a real /etc/named.user.conf.
export FUTRX_NAMED_INCLUDE_CANDIDATES="$TEST_TMP/named.user.conf"

# shellcheck source=../lib/bridge-dns.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib/bridge-dns.sh"

log()  { :; }
warn() { :; }
ok()   { :; }
err()  { :; }

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

BRIDGE_IP="10.221.94.1"

# ───────────────── identifying the squatter ─────────────────

SS_OUTPUT=""
ss() { printf '%s\n' "$SS_OUTPUT"; }

header='State  Recv-Q Send-Q Local Address:Port Peer Address:Port Process'

# The healthy case: LXD's own dnsmasq bound to the bridge address.
SS_OUTPUT="$header
UNCONN 0      0      ${BRIDGE_IP}:53        0.0.0.0:*    users:((\"dnsmasq\",pid=900,fd=6))"
[ "$(bridge_dns_listener "$BRIDGE_IP")" = "dnsmasq" ] \
    || fail "expected dnsmasq, got '$(bridge_dns_listener "$BRIDGE_IP")'"
bridge_dns_healthy "$BRIDGE_IP" || fail "expected a dnsmasq-served bridge to be healthy"

# BIND bound to the bridge address explicitly. This is what Plesk's DNS
# service does: listen-on { any; } enumerates interfaces and picks up lxdbr0
# whenever the bridge appears.
SS_OUTPUT="$header
UNCONN 0      0      ${BRIDGE_IP}:53        0.0.0.0:*    users:((\"named\",pid=800,fd=21))"
[ "$(bridge_dns_listener "$BRIDGE_IP")" = "named" ] \
    || fail "expected named, got '$(bridge_dns_listener "$BRIDGE_IP")'"
bridge_dns_healthy "$BRIDGE_IP" && fail "a BIND-held bridge address must not read as healthy"

# A wildcard listener owns the bridge address just as effectively as an
# explicit bind, and is the easier case to miss.
SS_OUTPUT="$header
UNCONN 0      0      0.0.0.0:53             0.0.0.0:*    users:((\"named\",pid=800,fd=21))"
[ "$(bridge_dns_listener "$BRIDGE_IP")" = "named" ] \
    || fail "expected a wildcard listener to count, got '$(bridge_dns_listener "$BRIDGE_IP")'"

# systemd-resolved's stub is on 127.0.0.53 and is not in the way.
SS_OUTPUT="$header
UNCONN 0      0      127.0.0.53%lo:53       0.0.0.0:*    users:((\"systemd-resolve\",pid=700,fd=13))"
[ -z "$(bridge_dns_listener "$BRIDGE_IP")" ] \
    || fail "the resolved stub must not be reported as holding the bridge address"

# dnsmasq on the bridge address and an unrelated resolver on the wildcard is a
# normal, healthy state — pi-hole and unbound both produce it. Taking whichever
# line `ss` printed first would report a working bridge as hijacked, and then
# either rewrite BIND's config or abort the install.
SS_OUTPUT="$header
UNCONN 0      0      0.0.0.0:53             0.0.0.0:*    users:((\"unbound\",pid=810,fd=9))
UNCONN 0      0      ${BRIDGE_IP}:53        0.0.0.0:*    users:((\"dnsmasq\",pid=900,fd=6))"
[ "$(bridge_dns_listener "$BRIDGE_IP")" = "dnsmasq" ] \
    || fail "an explicit bind must win over a wildcard one, got '$(bridge_dns_listener "$BRIDGE_IP")'"
bridge_dns_healthy "$BRIDGE_IP" || fail "dnsmasq on the bridge means healthy, whatever else is bound"

# A DNS listener on some other port is irrelevant.
SS_OUTPUT="$header
UNCONN 0      0      ${BRIDGE_IP}:5353      0.0.0.0:*    users:((\"avahi-daemon\",pid=600,fd=12))"
[ -z "$(bridge_dns_listener "$BRIDGE_IP")" ] \
    || fail "a listener on another port must not be reported"

# Nothing listening at all — LXD brought the bridge up but dnsmasq never bound.
SS_OUTPUT="$header"
[ -z "$(bridge_dns_listener "$BRIDGE_IP")" ] || fail "expected no listener"
bridge_dns_healthy "$BRIDGE_IP" && fail "an unserved bridge address must not read as healthy"

# ───────────────── the BIND exclusion ─────────────────

CHECKCONF_OK=1
named-checkconf() { [ "$CHECKCONF_OK" -eq 1 ]; }

INCLUDE="$TEST_TMP/named.user.conf"

# Writes the exclusion, and keeps whatever the operator already had in there.
rm -f "$INCLUDE"
printf '%s\n' '// operator settings' 'options { version none; };' > "$INCLUDE"
exclude_bridge_from_named "$BRIDGE_IP" || fail "exclusion should have succeeded"
grep -q "listen-on port 53 { !${BRIDGE_IP}; any; };" "$INCLUDE" \
    || fail "listen-on exclusion missing from $INCLUDE"
grep -q '// operator settings' "$INCLUDE" \
    || fail "pre-existing include content was lost"

# Idempotent: re-running the installer must not stack a second options block,
# which would be a config error rather than a no-op.
exclude_bridge_from_named "$BRIDGE_IP" || fail "second exclusion should have succeeded"
[ "$(grep -c "listen-on port 53" "$INCLUDE")" -eq 1 ] \
    || fail "exclusion was appended twice"

# A config BIND will not accept must roll back. A box whose BIND cannot start
# has lost its DNS service, which is far worse than the container problem.
rm -f "$INCLUDE"
printf '%s\n' '// operator settings' > "$INCLUDE"
CHECKCONF_OK=0
if exclude_bridge_from_named "$BRIDGE_IP"; then
    fail "expected non-zero when named-checkconf rejects the result"
fi
[ "$(cat "$INCLUDE")" = "// operator settings" ] \
    || fail "include was not rolled back: $(cat "$INCLUDE")"
grep -q "listen-on" "$INCLUDE" && fail "rolled-back include still carries the exclusion"

# Rollback when there was no include to begin with removes the file entirely,
# rather than leaving a broken one behind.
rm -f "$INCLUDE"
if exclude_bridge_from_named "$BRIDGE_IP"; then
    fail "expected non-zero when named-checkconf rejects the result"
fi
[ ! -e "$INCLUDE" ] || fail "a rejected include should not have been left on disk"

# ───────────────── end to end ─────────────────

CHECKCONF_OK=1
LXC_CALLS="$TEST_TMP/lxc-calls"
: > "$LXC_CALLS"
lxc() { printf '%s\n' "$*" >> "$LXC_CALLS"; }
systemctl() {
    # is-active for the BIND unit, restart for both.
    [ "${1:-}" = "is-active" ] && return 0
    return 0
}
sleep() { :; }

# Setting an LXD key to the value it already holds is a no-op — LXD
# short-circuits an update whose config did not change — so the bridge would
# never actually re-render and dnsmasq would never re-bind. The toggle has to
# be a real change, and has to land on true.
: > "$LXC_CALLS"
restart_bridge_dns lxdbr0 || fail "restart_bridge_dns should have succeeded"
[ "$(sed -n 1p "$LXC_CALLS")" = "network get lxdbr0 raw.dnsmasq" ] \
    || fail "expected the previous value to be read first, got: $(sed -n 1p "$LXC_CALLS")"
[ "$(sed -n 2p "$LXC_CALLS")" = "network set lxdbr0 raw.dnsmasq # remote.futrx: restarting dnsmasq" ] \
    || fail "expected a real config change, got: $(sed -n 2p "$LXC_CALLS")"
[ "$(sed -n 3p "$LXC_CALLS")" = "network unset lxdbr0 raw.dnsmasq" ] \
    || fail "expected the key cleared again, got: $(sed -n 3p "$LXC_CALLS")"

# Nothing it touches may be load-bearing for traffic. Toggling ipv4.nat would
# also force a re-render, but a run interrupted between the two writes would
# leave masquerading off — the exact "address but no egress" outage this file
# exists to diagnose, and worse than the state we started in.
grep -q "ipv4.nat" "$LXC_CALLS" && fail "the restart nudge must not touch NAT"

# An operator's own raw.dnsmasq must come back, not be cleared.
: > "$LXC_CALLS"
lxc() {
    if [ "$*" = "network get lxdbr0 raw.dnsmasq" ]; then
        printf 'dhcp-option=6,1.1.1.1\n'
        return 0
    fi
    printf '%s\n' "$*" >> "$LXC_CALLS"
}
restart_bridge_dns lxdbr0 || fail "restart_bridge_dns should have succeeded"
[ "$(sed -n 2p "$LXC_CALLS")" = "network set lxdbr0 raw.dnsmasq dhcp-option=6,1.1.1.1" ] \
    || fail "an existing raw.dnsmasq must be restored, got: $(sed -n 2p "$LXC_CALLS")"
lxc() { printf '%s\n' "$*" >> "$LXC_CALLS"; }

# The repair path: BIND holds the address, we exclude it, and dnsmasq takes
# over. The listener flips only after the restart, which is what proves the
# function re-checks rather than assuming.
rm -f "$INCLUDE"
SS_OUTPUT="$header
UNCONN 0      0      ${BRIDGE_IP}:53        0.0.0.0:*    users:((\"named\",pid=800,fd=21))"
restart_bridge_dns() {
    printf 'restart_bridge_dns %s\n' "$1" >> "$LXC_CALLS"
    SS_OUTPUT="$header
UNCONN 0      0      ${BRIDGE_IP}:53        0.0.0.0:*    users:((\"dnsmasq\",pid=901,fd=6))"
}
ensure_bridge_dns lxdbr0 "$BRIDGE_IP" || fail "expected the BIND repair to succeed"
grep -q "listen-on port 53" "$INCLUDE" || fail "repair did not write the exclusion"

# An already-healthy bridge is left completely alone.
: > "$LXC_CALLS"
rm -f "$INCLUDE"
ensure_bridge_dns lxdbr0 "$BRIDGE_IP" || fail "a healthy bridge should return zero"
[ ! -s "$LXC_CALLS" ] || fail "a healthy bridge should not be touched: $(cat "$LXC_CALLS")"
[ ! -e "$INCLUDE" ] || fail "a healthy bridge should not have edited BIND"

# An unrecognised squatter is reported, not silently worked around: guessing
# at somebody else's DNS server is how you take a production box offline.
SS_OUTPUT="$header
UNCONN 0      0      ${BRIDGE_IP}:53        0.0.0.0:*    users:((\"unbound\",pid=810,fd=9))"
if ensure_bridge_dns lxdbr0 "$BRIDGE_IP" 2>/dev/null; then
    fail "expected non-zero for an unrecognised listener"
fi

echo "PASS: bridge-dns"
