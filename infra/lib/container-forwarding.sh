# Container IPv4 forwarding through a restricted FORWARD chain.
#
# Docker sets `iptables -P FORWARD DROP` and adds ACCEPT rules only for its own
# bridges. LXD's masquerade rule for the container subnet still exists, but
# forwarded packets are dropped before they ever reach it, so containers get no
# IPv4 egress at all.
#
# Docker leaves ip6tables untouched, which makes the breakage nearly invisible:
# containers keep reaching every destination that publishes an AAAA record, and
# the first hard failure is whichever IPv4-only host the build happens to need.
# In practice that can be github.com during an optional Code Server install,
# reported as a bare connection timeout.
#
# The ACCEPT rules go in DOCKER-USER when Docker owns the ruleset: it is the
# chain Docker documents as user-owned, preserves across restarts, and
# evaluates before its own rules. Docker's isolation of its own bridges is
# left intact — only traffic on our bridge stops being collateral damage.
#
# Sourced by the installer and executed by the boot-time unit (see the
# --apply entry point at the bottom), so the rules are defined in one place.

# forward_policy_is_drop
# Succeeds when the FORWARD chain's default policy is DROP.
forward_policy_is_drop() {
    iptables -S FORWARD 2>/dev/null | grep -q '^-P FORWARD DROP'
}

# container_forwarding_chain
# Prints the chain the ACCEPT rules belong in: DOCKER-USER when Docker manages
# the firewall, otherwise FORWARD.
container_forwarding_chain() {
    if iptables -S DOCKER-USER >/dev/null 2>&1; then
        printf 'DOCKER-USER\n'
    else
        printf 'FORWARD\n'
    fi
}

# container_forwarding_needed
# Succeeds when the FORWARD path is restricted today, or when Docker is present
# and could restrict it on its next start.
container_forwarding_needed() {
    forward_policy_is_drop || iptables -S DOCKER-USER >/dev/null 2>&1
}

# ensure_container_forwarding <bridge>
# Idempotently accepts forwarded traffic to and from <bridge>. Prints one line
# per rule actually added and nothing when the rules are already in place, so
# callers can report only real changes. Returns non-zero when a needed rule
# could not be added.
ensure_container_forwarding() {
    local bridge="${1:-}" chain direction status=0
    if [ -z "$bridge" ]; then
        return 1
    fi
    chain="$(container_forwarding_chain)"
    for direction in -i -o; do
        if iptables -C "$chain" "$direction" "$bridge" -j ACCEPT 2>/dev/null; then
            continue
        fi
        if iptables -I "$chain" "$direction" "$bridge" -j ACCEPT 2>/dev/null; then
            printf '%s %s %s\n' "$chain" "$direction" "$bridge"
        else
            status=1
        fi
    done
    return "$status"
}

# Entry point for the boot-time unit: `bash container-forwarding.sh --apply
# <bridge>`. Self-gating, so the unit can be installed unconditionally and
# still do nothing on hosts where the FORWARD path is unrestricted.
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    case "${1:-}" in
        --apply)
            if container_forwarding_needed; then
                ensure_container_forwarding "${2:-lxdbr0}"
            fi
            ;;
        *)
            echo "usage: ${0} --apply <bridge>" >&2
            exit 2
            ;;
    esac
fi
