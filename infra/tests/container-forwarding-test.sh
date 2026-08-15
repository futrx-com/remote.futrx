#!/usr/bin/env bash
set -euo pipefail

TEST_TMP="$(mktemp -d)"
trap 'rm -rf "$TEST_TMP"' EXIT

# Point the library's two host-state paths at the sandbox before sourcing, so
# nothing in this suite can touch the real sysctl config or misread the real
# forwarding state of the machine running the tests.
export FUTRX_SYSCTL_FORWARD_CONF="$TEST_TMP/99-futrx-forward.conf"
export FUTRX_IP_FORWARD_PATH="$TEST_TMP/ip_forward"

# shellcheck source=../lib/container-forwarding.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib/container-forwarding.sh"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# A stub iptables backed by fake state, so the suite never touches the host
# firewall and can model a machine with or without Docker. Installed rules live
# in a file rather than a variable: callers capture output via command
# substitution, and a subshell's variable writes would not survive.
FORWARD_POLICY="DROP"
HAS_DOCKER_USER=1
INSERT_FAILS=0
RULES_FILE="$TEST_TMP/rules"

# One implementation, bound to three names. `iptables` is what a host with a
# single backend resolves to; the -nft / -legacy names let the dual-backend
# case drive both without the library finding real binaries.
_iptables_stub() {
    local self="$1"; shift
    case "$1" in
        -S)
            case "${2:-}" in
                FORWARD)
                    printf -- '-P FORWARD %s\n-A FORWARD -j DOCKER-USER\n' "$FORWARD_POLICY"
                    ;;
                DOCKER-USER)
                    [ "$HAS_DOCKER_USER" -eq 1 ] || return 1
                    printf -- '-N DOCKER-USER\n'
                    ;;
            esac
            ;;
        -C)
            grep -qxF "$self $2 $3 $4" "$RULES_FILE" || return 1
            ;;
        -I)
            [ "$INSERT_FAILS" -eq 0 ] || return 1
            printf '%s\n' "$self $2 $3 $4" >> "$RULES_FILE"
            ;;
        *)
            return 1
            ;;
    esac
}
iptables()         { _iptables_stub iptables "$@"; }
iptables-nft()     { _iptables_stub iptables-nft "$@"; }
iptables-legacy()  { _iptables_stub iptables-legacy "$@"; }

reset_state() {
    FORWARD_POLICY="${1:-DROP}"
    HAS_DOCKER_USER="${2:-1}"
    INSERT_FAILS=0
    # Default to the single-backend host. Auto-detection would otherwise find
    # all three stubs, since `command -v` resolves shell functions — which is
    # itself worth knowing: on a real Ubuntu 24.04 box both iptables-nft and
    # iptables-legacy exist and both get the rules.
    export FUTRX_IPTABLES_BACKENDS="iptables"
    : > "$RULES_FILE"
}

# ───────────────── chain selection ─────────────────

# Docker present and restricting FORWARD: the rules belong in the chain Docker
# preserves for us, and the reporting predicate agrees work was outstanding.
reset_state DROP 1
container_forwarding_needed || fail "expected work to be reported with FORWARD DROP"
[ "$(container_forwarding_chain)" = "DOCKER-USER" ] \
    || fail "expected DOCKER-USER, got '$(container_forwarding_chain)'"

added="$(ensure_container_forwarding lxdbr0)" || fail "ensure returned non-zero"
[ "$(printf '%s\n' "$added" | grep -c .)" -eq 2 ] \
    || fail "expected two rules to be added, got: $added"
printf '%s\n' "$added" | grep -q -- "-i lxdbr0" || fail "inbound rule missing: $added"
printf '%s\n' "$added" | grep -q -- "-o lxdbr0" || fail "outbound rule missing: $added"

# Re-running must be silent: the installer reports only real changes, and a
# re-run must not stack duplicate rules on every update.
added="$(ensure_container_forwarding lxdbr0)" || fail "second ensure returned non-zero"
[ -z "$added" ] || fail "expected no changes on re-run, got: $added"

# No Docker: the rules go straight to FORWARD.
reset_state ACCEPT 0
[ "$(container_forwarding_chain)" = "FORWARD" ] \
    || fail "expected FORWARD, got '$(container_forwarding_chain)'"

# ───────────────── unconditional apply ─────────────────

# The regression that shipped: an unrestricted-looking host without Docker
# reported "no work needed", the installer skipped the whole step, and the
# base-image build failed identically on every re-run because the real blocker
# was a rule (or another backend, or a sysctl) that the probe could not see.
# Detection no longer gates the apply path.
reset_state ACCEPT 0
if container_forwarding_needed; then
    fail "expected the reporting predicate to see an unrestricted host"
fi
added="$(ensure_container_forwarding lxdbr0)" || fail "ensure returned non-zero"
[ "$(printf '%s\n' "$added" | grep -c .)" -eq 2 ] \
    || fail "expected rules to be applied even when detection sees no restriction, got: $added"
printf '%s\n' "$added" | grep -q "^iptables FORWARD -i lxdbr0$" \
    || fail "expected the rule in FORWARD, got: $added"

# A host firewall that drops FORWARD without Docker still needs the rules.
reset_state DROP 0
container_forwarding_needed || fail "expected work with FORWARD DROP and no Docker"
added="$(ensure_container_forwarding lxdbr0)" || fail "ensure returned non-zero"
printf '%s\n' "$added" | grep -q "^iptables FORWARD -i lxdbr0$" \
    || fail "expected the rule in FORWARD, got: $added"

# ───────────────── both iptables backends ─────────────────

# The kernel consults nft and legacy tables both, so a DROP in either one wins.
# Accepting in only the backend `iptables` happens to point at leaves the other
# one dropping, which looks exactly like the rules having had no effect.
reset_state DROP 0
export FUTRX_IPTABLES_BACKENDS="iptables-nft iptables-legacy"
added="$(ensure_container_forwarding lxdbr0)" || fail "ensure returned non-zero"
[ "$(printf '%s\n' "$added" | grep -c .)" -eq 4 ] \
    || fail "expected four rules across two backends, got: $added"
for bin in iptables-nft iptables-legacy; do
    for dir in -i -o; do
        printf '%s\n' "$added" | grep -qxF "$bin FORWARD $dir lxdbr0" \
            || fail "missing '$bin FORWARD $dir lxdbr0' in: $added"
    done
done
# Idempotent across backends too.
added="$(ensure_container_forwarding lxdbr0)" || fail "second dual-backend ensure returned non-zero"
[ -z "$added" ] || fail "expected no changes on dual-backend re-run, got: $added"

# Auto-detection finds every backend that answers, and falls back to plain
# `iptables` when none of the versioned names resolve.
reset_state DROP 0
unset FUTRX_IPTABLES_BACKENDS
[ "$(container_forwarding_backends | tr '\n' ' ')" = "iptables-nft iptables-legacy " ] \
    || fail "expected both backends detected, got: $(container_forwarding_backends | tr '\n' ' ')"

# ───────────────── failure surfaces ─────────────────

# A refused insert must surface, not be silently swallowed — otherwise the
# build proceeds and dies minutes later with an unrelated-looking timeout.
reset_state DROP 1
INSERT_FAILS=1
if ensure_container_forwarding lxdbr0 >/dev/null 2>&1; then
    fail "expected non-zero when the rule could not be added"
fi

# A missing bridge name is a caller bug, not a no-op.
reset_state DROP 1
if ensure_container_forwarding "" >/dev/null 2>&1; then
    fail "expected non-zero for an empty bridge name"
fi

# ───────────────── ip_forward ─────────────────

sysctl() {
    # `sysctl -qw net.ipv4.ip_forward=1`
    [ "${1:-}" = "-qw" ] || return 1
    printf '%s\n' "${2#*=}" > "$FUTRX_IP_FORWARD_PATH"
}

# IPv4 forwarding off is the blocker that most resembles the symptom:
# net.ipv6.conf.all.forwarding is a separate knob, so containers keep reaching
# every AAAA destination and only IPv4-only hosts fail.
echo 0 > "$FUTRX_IP_FORWARD_PATH"
rm -f "$FUTRX_SYSCTL_FORWARD_CONF"
changed="$(ensure_ip_forwarding)" || fail "ensure_ip_forwarding returned non-zero"
[ -n "$changed" ] || fail "expected ensure_ip_forwarding to report a change"
[ "$(cat "$FUTRX_IP_FORWARD_PATH")" = "1" ] || fail "ip_forward was not turned on"
grep -q '^net\.ipv4\.ip_forward = 1$' "$FUTRX_SYSCTL_FORWARD_CONF" \
    || fail "ip_forward was not persisted to $FUTRX_SYSCTL_FORWARD_CONF"

# Already on and already persisted: silent, so the installer reports only real
# changes.
changed="$(ensure_ip_forwarding)" || fail "second ensure_ip_forwarding returned non-zero"
[ -z "$changed" ] || fail "expected no report when ip_forward was already set, got: $changed"

# Live setting on but the drop-in missing: a distro or Plesk sysctl file can
# set it back to 0 on the next boot, so persisting is still a real change.
rm -f "$FUTRX_SYSCTL_FORWARD_CONF"
changed="$(ensure_ip_forwarding)" || fail "ensure_ip_forwarding returned non-zero"
[ -n "$changed" ] || fail "expected a report when only the drop-in was missing"
[ -r "$FUTRX_SYSCTL_FORWARD_CONF" ] || fail "drop-in was not rewritten"

# ───────────────── native nftables drops ─────────────────

NFT_RULESET=""
nft() {
    [ "${1:-}" = "list" ] || return 1
    printf '%s\n' "$NFT_RULESET"
}

# The iptables compatibility tables are the ones our own ACCEPT rules land in,
# so a drop there is already handled and must not be reported as unfixable.
NFT_RULESET='table ip filter {
	chain FORWARD {
		type filter hook forward priority filter; policy drop;
	}
}'
[ -z "$(detect_nft_forward_drop)" ] \
    || fail "ip/filter is the iptables-nft table we fix; it must not be reported"

# A native table is a different matter: in nftables every table sees the packet
# and any drop wins, so an accept of our own cannot rescue it. Report, never
# claim a fix.
NFT_RULESET='table inet plesk_fw {
	chain forward {
		type filter hook forward priority 0; policy drop;
	}
}'
[ "$(detect_nft_forward_drop)" = "inet plesk_fw forward" ] \
    || fail "expected the native table reported, got: '$(detect_nft_forward_drop)'"

# An accepting native table is not a problem.
NFT_RULESET='table inet plesk_fw {
	chain forward {
		type filter hook forward priority 0; policy accept;
	}
}'
[ -z "$(detect_nft_forward_drop)" ] || fail "an accepting forward chain must not be reported"

echo "PASS: container-forwarding"
