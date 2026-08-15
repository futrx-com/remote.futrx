#!/usr/bin/env bash
#
# Why can't my containers reach IPv4?
#
# Run this when a build fails with "the builder container cannot reach any
# IPv4 address", or any time an agent reports "Network unreachable".
#
#   sudo bash /opt/remote.futrx/infra/diagnose-network.sh
#
# It changes nothing. It collects, in one pass, every piece of state that can
# produce that symptom, and ends with a verdict naming the most likely cause.
# The point is to stop guessing: the same message has at least five distinct
# causes, and the fix for one does nothing for the others.
#
# Flags:
#   --no-probe   skip launching a throwaway container (faster, less conclusive)
set -uo pipefail

BRIDGE="${1:-lxdbr0}"
[ "${BRIDGE#--}" = "$BRIDGE" ] || BRIDGE="lxdbr0"
PROBE=1
for a in "$@"; do
    [ "$a" = "--no-probe" ] && PROBE=0
done

INFRA_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
PROBE_CONTAINER="futrx-netdiag"

bold()  { printf '\n\033[1;36m── %s\033[0m\n' "$*"; }
good()  { printf '\033[1;32m  ✓\033[0m %s\n' "$*"; }
bad()   { printf '\033[1;31m  ✗\033[0m %s\n' "$*"; }
note()  { printf '    %s\n' "$*"; }

# Findings accumulate as one-line verdicts, printed together at the end. A
# 200-line dump nobody reads is how the original error message failed.
FINDINGS=()
finding() { FINDINGS+=("$*"); }

if [ "$EUID" -ne 0 ]; then
    echo "this needs root (iptables and lxc both do); rerun with sudo" >&2
    exit 1
fi

export PATH="/snap/bin:$PATH"

# shellcheck source=lib/container-forwarding.sh
. "$INFRA_DIR/lib/container-forwarding.sh"
# shellcheck source=lib/bridge-dns.sh
. "$INFRA_DIR/lib/bridge-dns.sh"
# shellcheck source=lib/plesk.sh
. "$INFRA_DIR/lib/plesk.sh"

# ───────────────── host ─────────────────
bold "Host"
note "$(uname -sr)  $( . /etc/os-release 2>/dev/null && echo "$PRETTY_NAME" )"
if plesk_present; then
    good "Plesk $(plesk_version 2>/dev/null || echo detected) — its nginx, BIND and firewall all overlap with ours"
else
    note "no Plesk"
fi

# ───────────────── IPv4 forwarding ─────────────────
bold "IPv4 forwarding"
IP_FORWARD="$(cat /proc/sys/net/ipv4/ip_forward 2>/dev/null || echo '?')"
IP6_FORWARD="$(cat /proc/sys/net/ipv6/conf/all/forwarding 2>/dev/null || echo '?')"
if [ "$IP_FORWARD" = "1" ]; then
    good "net.ipv4.ip_forward = 1"
else
    bad "net.ipv4.ip_forward = $IP_FORWARD"
    finding "net.ipv4.ip_forward is off. This is the cause that most resembles the symptom: IPv6 forwarding is a separate knob (currently $IP6_FORWARD), so containers keep reaching every AAAA destination and only IPv4-only hosts fail."
fi
note "net.ipv6.conf.all.forwarding = $IP6_FORWARD"
for f in /etc/sysctl.conf /etc/sysctl.d/*.conf /usr/lib/sysctl.d/*.conf; do
    [ -r "$f" ] || continue
    if grep -qE '^[[:space:]]*net\.ipv4\.ip_forward[[:space:]]*=[[:space:]]*0' "$f" 2>/dev/null; then
        bad "$f sets ip_forward=0 and will win on the next boot"
        finding "$f disables IPv4 forwarding on boot; our drop-in must sort after it."
    fi
done

# ───────────────── iptables ─────────────────
bold "iptables"
BACKENDS="$(container_forwarding_backends)"
note "backends the kernel consults: $(printf '%s' "$BACKENDS" | tr '\n' ' ')"
for bin in $BACKENDS; do
    policy="$("$bin" -S FORWARD 2>/dev/null | grep '^-P FORWARD' | awk '{print $3}')"
    if [ "$policy" = "DROP" ]; then
        bad "$bin: FORWARD policy is DROP"
    else
        good "$bin: FORWARD policy is ${policy:-unknown}"
    fi
    if "$bin" -S DOCKER-USER >/dev/null 2>&1; then
        note "  DOCKER-USER exists (Docker owns this ruleset)"
    else
        note "  no DOCKER-USER chain (Docker is not managing this backend)"
    fi
    for direction in -i -o; do
        chain="$(container_forwarding_chain "$bin")"
        if "$bin" -C "$chain" "$direction" "$BRIDGE" -j ACCEPT 2>/dev/null; then
            good "  $chain $direction $BRIDGE ACCEPT present"
        else
            bad "  $chain $direction $BRIDGE ACCEPT MISSING"
            finding "$bin is missing the '$chain $direction $BRIDGE ACCEPT' rule. Re-run infra/install.sh, or apply it now with: $bin -I $chain $direction $BRIDGE -j ACCEPT"
        fi
    done
    # A blanket reject partway down FORWARD drops our traffic even when the
    # policy reads ACCEPT — the case the old policy-only check could not see.
    if "$bin" -S FORWARD 2>/dev/null | grep -qE '^-A FORWARD (-j (DROP|REJECT)|-m .* -j (DROP|REJECT))$'; then
        note "  FORWARD carries an unconditional DROP/REJECT rule:"
        "$bin" -S FORWARD 2>/dev/null | grep -E '^-A FORWARD .*-j (DROP|REJECT)' | sed 's/^/      /'
    fi
done

# ───────────────── native nftables ─────────────────
bold "nftables"
if command -v nft >/dev/null 2>&1; then
    NFT_DROPS="$(detect_nft_forward_drop)"
    if [ -n "$NFT_DROPS" ]; then
        while read -r family table chain; do
            [ -n "$family" ] || continue
            bad "table $family $table chain $chain hooks forward with policy drop"
            finding "A native nftables table ($family $table / $chain) drops forwarded traffic. Every table sees the packet and any drop wins, so our iptables ACCEPT rules cannot override it. Add an accept in that table: nft insert rule $family $table $chain iifname \"$BRIDGE\" accept; nft insert rule $family $table $chain oifname \"$BRIDGE\" accept"
        done <<< "$NFT_DROPS"
    else
        good "no native forward-hook drop outside the iptables compatibility tables"
    fi
else
    note "nft not installed"
fi

# ───────────────── the bridge ─────────────────
bold "LXD bridge ($BRIDGE)"
if ! command -v lxc >/dev/null 2>&1; then
    bad "lxc not on PATH — LXD is not installed or /snap/bin is missing"
    finding "LXD is not reachable from this shell; nothing below could be checked."
    BRIDGE_IP=""
elif ! lxc network show "$BRIDGE" >/dev/null 2>&1; then
    bad "network '$BRIDGE' does not exist"
    finding "The '$BRIDGE' bridge is missing entirely — run 'lxd init --auto' or re-run infra/install.sh."
    BRIDGE_IP=""
else
    BRIDGE_ADDR="$(lxc network get "$BRIDGE" ipv4.address 2>/dev/null)"
    BRIDGE_NAT="$(lxc network get "$BRIDGE" ipv4.nat 2>/dev/null)"
    BRIDGE_IP="${BRIDGE_ADDR%%/*}"
    if [ -z "$BRIDGE_ADDR" ] || [ "$BRIDGE_ADDR" = "none" ]; then
        bad "ipv4.address = ${BRIDGE_ADDR:-unset}"
        finding "The bridge has no IPv4 subnet, so containers can never get a lease. Fix: lxc network set $BRIDGE ipv4.address auto"
        BRIDGE_IP=""
    else
        good "ipv4.address = $BRIDGE_ADDR"
    fi
    if [ "$BRIDGE_NAT" = "true" ]; then
        good "ipv4.nat = true"
    else
        bad "ipv4.nat = ${BRIDGE_NAT:-unset}"
        finding "The bridge does not masquerade, so container packets leave with an unroutable source address and nothing comes back. Fix: lxc network set $BRIDGE ipv4.nat true"
    fi
fi

# ───────────────── DHCP/DNS on the bridge ─────────────────
bold "DHCP + DNS on the bridge"
if [ -z "${BRIDGE_IP:-}" ]; then
    note "skipped — no bridge address"
else
    HOLDER="$(bridge_dns_listener "$BRIDGE_IP" || true)"
    if [ "$HOLDER" = "dnsmasq" ]; then
        good "dnsmasq owns ${BRIDGE_IP}:53"
    elif [ -n "$HOLDER" ]; then
        bad "$HOLDER owns ${BRIDGE_IP}:53, not dnsmasq"
        finding "$HOLDER holds ${BRIDGE_IP}:53, so LXD's dnsmasq could not bind and containers get no DHCP lease. They come up with IPv6 only, which looks exactly like a firewall problem but is not — no forwarding fix will help. Re-run infra/install.sh: it excludes the bridge from BIND automatically when Plesk is present."
    else
        bad "nothing is listening on ${BRIDGE_IP}:53"
        finding "No DHCP/DNS server on ${BRIDGE_IP}:53 — containers will never receive an IPv4 lease. Check 'systemctl status snap.lxd.daemon' and the LXD log for a dnsmasq bind failure."
    fi
    ss -lnup 2>/dev/null | awk 'NR==1 || /:53 /' | sed 's/^/    /'
fi

# ───────────────── containers ─────────────────
bold "Containers"
if command -v lxc >/dev/null 2>&1; then
    lxc list -c ns4 --format csv 2>/dev/null | sed 's/ (eth0)//' | while IFS=, read -r name state ipv4; do
        [ -n "$name" ] || continue
        if [ "$state" != "RUNNING" ]; then
            note "$name: $state"
        elif [ -z "$ipv4" ]; then
            bad "$name: RUNNING with no IPv4"
        else
            good "$name: $ipv4"
        fi
    done
    if lxc list -c ns4 --format csv 2>/dev/null | sed 's/ (eth0)//' \
        | awk -F, '$2 == "RUNNING" && $3 == ""' | grep -q .; then
        finding "Running containers have no IPv4 address at all — that is a DHCP problem (see the bridge DNS section), not a forwarding one."
    fi
else
    note "skipped — lxc unavailable"
fi

# ───────────────── reachability ─────────────────
bold "Reachability"
if timeout 8 bash -c 'exec 3<>/dev/tcp/1.1.1.1/443' 2>/dev/null; then
    good "host → 1.1.1.1:443"
else
    bad "host → 1.1.1.1:443 failed"
    finding "The host itself cannot reach IPv4. This is upstream of anything containers do — check the provider's firewall and the host's default route."
fi

if [ "$PROBE" -eq 1 ] && command -v lxc >/dev/null 2>&1 && [ -n "${BRIDGE_IP:-}" ]; then
    note "launching a throwaway container (~15s; --no-probe to skip)"
    lxc delete -f "$PROBE_CONTAINER" >/dev/null 2>&1 || true
    if lxc launch ubuntu:24.04 "$PROBE_CONTAINER" >/dev/null 2>&1; then
        sleep 12
        PROBE_ADDR="$(lxc exec "$PROBE_CONTAINER" -- ip -4 -o addr show eth0 2>/dev/null | awk '{print $4}')"
        PROBE_ROUTE="$(lxc exec "$PROBE_CONTAINER" -- ip -4 route show default 2>/dev/null | head -1)"
        if [ -n "$PROBE_ADDR" ]; then
            good "probe container address: $PROBE_ADDR"
        else
            bad "probe container got no IPv4 address"
            finding "A fresh container receives no IPv4 lease — confirms the DHCP path, not the firewall, is what is broken."
        fi
        [ -n "$PROBE_ROUTE" ] && good "default route: $PROBE_ROUTE" || bad "no IPv4 default route"
        if lxc exec "$PROBE_CONTAINER" -- timeout 8 bash -c 'exec 3<>/dev/tcp/1.1.1.1/443' >/dev/null 2>&1; then
            good "probe container → 1.1.1.1:443"
        else
            bad "probe container → 1.1.1.1:443 failed"
            if [ -n "$PROBE_ADDR" ]; then
                finding "The container has an address and a route but cannot reach IPv4 — that is the forwarding/NAT path. See the iptables and nftables sections above."
            fi
        fi
        lxc delete -f "$PROBE_CONTAINER" >/dev/null 2>&1 || true
    else
        note "could not launch the probe container (no ubuntu:24.04 image cached?)"
    fi
fi

# ───────────────── verdict ─────────────────
printf '\n\033[1;36m── Verdict\033[0m\n'
if [ "${#FINDINGS[@]}" -eq 0 ]; then
    good "Nothing wrong found. If builds still fail, capture the failing stage's"
    note "output and check whether the destination is reachable from the host at all."
else
    for f in "${FINDINGS[@]}"; do
        printf '\n\033[1;31m  •\033[0m %s\n' "$f"
    done
    printf '\n'
fi
