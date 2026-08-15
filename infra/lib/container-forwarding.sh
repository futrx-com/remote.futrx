# Container IPv4 egress: keep the LXD bridge's forwarded traffic alive.
#
# The failure this prevents is nearly invisible. IPv4 forwarding dies while
# IPv6 keeps working, so containers still reach every destination that
# publishes an AAAA record — apt, NodeSource, the npm registry and Google's
# CDN all succeed — and the first hard failure is whichever IPv4-only host the
# build happens to need. In practice that is github.com in the browser IDE
# stage, several minutes in, reported as a bare connection timeout.
#
# Four separate things can break it, and earlier versions of this file only
# knew about the first:
#
#   1. Docker sets `iptables -P FORWARD DROP` and accepts only its own
#      bridges. LXD's masquerade rule still exists, but forwarded packets are
#      dropped before they ever reach it.
#   2. Any other host firewall (Plesk's firewall module, a hardening profile,
#      a hand-rolled ruleset) does the same thing with a *rule* rather than a
#      policy, which the old policy-only probe could not see.
#   3. `net.ipv4.ip_forward=0`. This is the one that most looks like the
#      symptom: `net.ipv6.conf.all.forwarding` is a separate knob, so IPv6
#      survives untouched.
#   4. The rules exist in one iptables backend and the DROP is in the other.
#      Ubuntu 24.04 ships iptables-nft, but legacy tables can still be
#      populated, and the kernel consults both — a DROP in either one wins.
#
# So this file no longer tries to infer whether work is needed. The ACCEPT
# rules are scoped to our own bridge, idempotent, and harmless on a host that
# was never restricted, so they are applied unconditionally. Guessing wrong
# cost us a silent skip and an install that failed identically on every
# re-run; applying unconditionally costs two rules.
#
# The ACCEPT rules go in DOCKER-USER when Docker owns the ruleset: it is the
# chain Docker documents as user-owned, preserves across restarts, and
# evaluates before its own rules. Docker's isolation of its own bridges is
# left intact — only traffic on our bridge stops being collateral damage.
#
# What this file deliberately does NOT do is fix a native nftables drop. In
# nftables every table sees the packet and any drop wins, so an accept in a
# table of our own would not rescue one in somebody else's.
# detect_nft_forward_drop reports those precisely instead of claiming a fix
# that did not happen.
#
# Sourced by the installer and executed by the boot-time unit and its
# re-apply timer (see the --apply entry point at the bottom), so the rules are
# defined in one place.

FUTRX_SYSCTL_FORWARD_CONF="${FUTRX_SYSCTL_FORWARD_CONF:-/etc/sysctl.d/99-futrx-forward.conf}"
FUTRX_IP_FORWARD_PATH="${FUTRX_IP_FORWARD_PATH:-/proc/sys/net/ipv4/ip_forward}"

# container_forwarding_backends
# Prints one iptables binary per line — every backend whose ruleset the kernel
# actually consults. Ubuntu 24.04's `iptables` is a symlink to iptables-nft,
# but a box that has been through Docker, Plesk or a distro upgrade can carry
# rules in the legacy tables too, and a DROP in either backend drops the
# packet. Overridable for tests, which stub the binaries as shell functions.
container_forwarding_backends() {
    if [ -n "${FUTRX_IPTABLES_BACKENDS:-}" ]; then
        printf '%s\n' $FUTRX_IPTABLES_BACKENDS
        return 0
    fi
    local found=0 bin
    for bin in iptables-nft iptables-legacy; do
        command -v "$bin" >/dev/null 2>&1 || continue
        # An empty backend is not worth touching: it has no policy or rule
        # that could drop anything.
        if "$bin" -S FORWARD >/dev/null 2>&1; then
            printf '%s\n' "$bin"
            found=1
        fi
    done
    [ "$found" -eq 1 ] || printf 'iptables\n'
}

# forward_policy_is_drop [binary]
# Succeeds when the FORWARD chain's default policy is DROP.
forward_policy_is_drop() {
    local bin="${1:-iptables}"
    "$bin" -S FORWARD 2>/dev/null | grep -q '^-P FORWARD DROP'
}

# container_forwarding_chain [binary]
# Prints the chain the ACCEPT rules belong in: DOCKER-USER when Docker manages
# the firewall, otherwise FORWARD.
container_forwarding_chain() {
    local bin="${1:-iptables}"
    if "$bin" -S DOCKER-USER >/dev/null 2>&1; then
        printf 'DOCKER-USER\n'
    else
        printf 'FORWARD\n'
    fi
}

# container_forwarding_needed [binary]
# Reporting only — the apply path no longer gates on this. Succeeds when the
# FORWARD path is visibly restricted today, or when Docker is present and
# could restrict it on its next start. A false answer here now costs a log
# line rather than an unusable install.
container_forwarding_needed() {
    local bin="${1:-iptables}"
    forward_policy_is_drop "$bin" || "$bin" -S DOCKER-USER >/dev/null 2>&1
}

# ensure_ip_forwarding
# Turns on IPv4 forwarding and persists it. Prints a line when it had to
# change something, nothing when it was already on. Returns non-zero when the
# live setting could not be applied.
#
# Persisted separately from the live write because a sysctl drop-in that ships
# with the distro or with Plesk can set it back to 0 on the next boot or on
# the next `sysctl --system`, and 99- sorts after those.
ensure_ip_forwarding() {
    local current changed=""
    current="$(cat "$FUTRX_IP_FORWARD_PATH" 2>/dev/null || echo 0)"
    if [ "$current" != "1" ]; then
        sysctl -qw net.ipv4.ip_forward=1 2>/dev/null || return 1
        changed="net.ipv4.ip_forward=1"
    fi
    if [ ! -r "$FUTRX_SYSCTL_FORWARD_CONF" ] \
       || ! grep -q '^net\.ipv4\.ip_forward *= *1' "$FUTRX_SYSCTL_FORWARD_CONF" 2>/dev/null; then
        mkdir -p "$(dirname "$FUTRX_SYSCTL_FORWARD_CONF")" 2>/dev/null || true
        printf '%s\n' \
            '# Managed by remote.futrx (infra/lib/container-forwarding.sh).' \
            '# LXD containers forward their IPv4 through the host; without this they' \
            '# reach only IPv6, which fails late and looks like an unrelated timeout.' \
            'net.ipv4.ip_forward = 1' > "$FUTRX_SYSCTL_FORWARD_CONF" 2>/dev/null || return 1
        changed="${changed:-net.ipv4.ip_forward=1} (persisted)"
    fi
    [ -z "$changed" ] || printf '%s\n' "$changed"
    return 0
}

# ensure_bridge_nat <bridge>
# Verifies the LXD bridge actually has an IPv4 subnet and masquerades it.
# Without ipv4.address the containers never get a lease; without ipv4.nat
# their packets leave with an unroutable source. Prints a line per change.
# Install-time only — the boot unit skips it, since `lxc` may not be on PATH
# there and neither setting drifts on its own.
ensure_bridge_nat() {
    local bridge="${1:-}" address nat
    [ -n "$bridge" ] || return 1
    command -v lxc >/dev/null 2>&1 || return 0

    address="$(lxc network get "$bridge" ipv4.address 2>/dev/null || true)"
    if [ -z "$address" ] || [ "$address" = "none" ]; then
        lxc network set "$bridge" ipv4.address auto >/dev/null 2>&1 || return 1
        printf '%s ipv4.address=auto\n' "$bridge"
    fi

    nat="$(lxc network get "$bridge" ipv4.nat 2>/dev/null || true)"
    if [ "$nat" != "true" ]; then
        lxc network set "$bridge" ipv4.nat true >/dev/null 2>&1 || return 1
        printf '%s ipv4.nat=true\n' "$bridge"
    fi
    return 0
}

# detect_nft_forward_drop
# Prints one "<family> <table> <chain>" line per native-nftables chain that
# hooks forward with a drop policy. The iptables compatibility tables
# (ip/ip6 filter) are excluded: those are the ones our own ACCEPT rules land
# in and actually fix. Anything else needs a rule in that table, which only
# its owner can safely add.
detect_nft_forward_drop() {
    command -v nft >/dev/null 2>&1 || return 0
    # The family/table and the hook/policy live on different lines, so track
    # the enclosing block as we go rather than matching a single line.
    nft list ruleset 2>/dev/null | awk '
        $1 == "table" { family = $2; table = $3; next }
        $1 == "chain" { chain = $2; next }
        /hook forward/ && /policy drop/ {
            if (family == "" || table == "" || chain == "") next
            if ((family == "ip" || family == "ip6") && table == "filter") next
            print family, table, chain
        }
    '
}

# ensure_container_forwarding <bridge>
# Idempotently accepts forwarded traffic to and from <bridge>, in every
# backend the kernel consults. Prints one "<binary> <chain> <direction>
# <bridge>" line per rule actually added and nothing when everything is
# already in place, so callers can report only real changes. Returns non-zero
# when a needed rule could not be added.
ensure_container_forwarding() {
    local bridge="${1:-}" bin chain direction status=0
    if [ -z "$bridge" ]; then
        return 1
    fi
    for bin in $(container_forwarding_backends); do
        chain="$(container_forwarding_chain "$bin")"
        for direction in -i -o; do
            if "$bin" -C "$chain" "$direction" "$bridge" -j ACCEPT 2>/dev/null; then
                continue
            fi
            if "$bin" -I "$chain" "$direction" "$bridge" -j ACCEPT 2>/dev/null; then
                printf '%s %s %s %s\n' "$bin" "$chain" "$direction" "$bridge"
            else
                status=1
            fi
        done
    done
    return "$status"
}

# Entry point for the boot-time unit and its re-apply timer:
# `bash container-forwarding.sh --apply <bridge>`. Runs unconditionally — a
# host firewall that reconciles its ruleset (Plesk's firewall module and
# fail2ban both do) silently removes our rules mid-life, and Docker reinstates
# its policy on every start.
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    case "${1:-}" in
        --apply)
            ensure_ip_forwarding || true
            ensure_container_forwarding "${2:-lxdbr0}"
            ;;
        *)
            echo "usage: ${0} --apply <bridge>" >&2
            exit 2
            ;;
    esac
fi
