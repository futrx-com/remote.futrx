#!/usr/bin/env bash
set -euo pipefail

INFRA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
. "$INFRA_DIR/lib/container-api-dns.sh"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT
STATE="$TEST_DIR/raw.dnsmasq"
CALLS="$TEST_DIR/calls"

fail() { echo "FAIL: $*" >&2; exit 1; }
sleep() { :; }
reset_state() {
    : > "$STATE"
    : > "$CALLS"
    GET_FAIL=0 SET_FAIL=0 DIG_FAIL=0 DNS_STATUS=NOERROR
    A_ANSWER='remote.example.com. 0 IN A 10.8.0.1'
    AAAA_ANSWER=''
}
lxc() {
    [ "$1 $2 $3 $4" = 'network get lxdbr0 raw.dnsmasq' ] ||
        [ "$1 $2 $3 $4" = 'network set lxdbr0 raw.dnsmasq' ] || fail "unexpected lxc call"
    case "$2" in
        get) [ "$GET_FAIL" -eq 0 ] || return 1; cat "$STATE" ;;
        set) [ "$SET_FAIL" -eq 0 ] || return 1
             printf '%s' "$5" > "$STATE"; echo set >> "$CALLS" ;;
    esac
}
dig() {
    [ "$1 $2" = '@10.8.0.1 remote.example.com.' ] || fail "wrong DNS server or hostname"
    [ "$DIG_FAIL" -eq 0 ] || return 1
    printf ';; ->>HEADER<<- opcode: QUERY, status: %s, id: 1\n' "$DNS_STATUS"
    case "$3" in
        A) printf '%s\n' "$A_ANSWER" ;;
        AAAA) printf '%s\n' "$AAAA_ANSWER" ;;
        *) fail "unexpected query type" ;;
    esac
}

# Fresh install, preserved custom settings, and idempotent update.
reset_state
printf '%s' $'# operator configuration\nserver=/private.example/10.8.0.2\nhost-record=other.example,10.8.0.3' > "$STATE"
original="$(cat "$STATE")"
ensure_container_api_dns lxdbr0 remote.example.com 10.8.0.1 || fail "happy path"
expected="$original"$'\n# BEGIN remote.futrx container API DNS\nno-hosts\nhost-record=remote.example.com,10.8.0.1\n# END remote.futrx container API DNS'
[ "$(cat "$STATE")" = "$expected" ] || fail "custom settings changed"
ensure_container_api_dns lxdbr0 remote.example.com 10.8.0.1 || fail "idempotent run"
[ "$(wc -l < "$CALLS")" -eq 1 ] || fail "unchanged config was reapplied"

# Renamed host or changed bridge address replaces the old managed block.
sed -i 's/remote.example.com,10.8.0.1/old.example.com,10.9.0.1/' "$STATE"
ensure_container_api_dns lxdbr0 remote.example.com 10.8.0.1 || fail "replace old pin"
[ "$(cat "$STATE")" = "$expected" ] || fail "old managed record survived"

# Failed answers stop convergence and restore existing custom configuration.
for mode in loopback mixed public empty ipv6 servfail timeout; do
    reset_state
    printf '%s' "$original" > "$STATE"
    case "$mode" in
        loopback) A_ANSWER='remote.example.com. 0 IN A 127.0.1.1' ;;
        mixed) A_ANSWER+=$'\nremote.example.com. 0 IN A 127.0.1.1' ;;
        public) A_ANSWER='remote.example.com. 0 IN A 203.0.113.1' ;;
        empty) A_ANSWER='' ;;
        ipv6) AAAA_ANSWER='remote.example.com. 0 IN AAAA ::1' ;;
        servfail) DNS_STATUS=SERVFAIL ;;
        timeout) DIG_FAIL=1 ;;
    esac
    if ensure_container_api_dns lxdbr0 remote.example.com 10.8.0.1 >"$TEST_DIR/out" 2>&1; then
        fail "accepted $mode DNS"
    fi
    [ "$(cat "$STATE")" = "$original" ] || fail "$mode did not restore custom config"
    grep -q 'Bridge DNS verification failed' "$TEST_DIR/out" || fail "unclear $mode error"
done

for mode in read write marker address loopback-address invalid-address hostname; do
    reset_state
    address=10.8.0.1 hostname=remote.example.com
    case "$mode" in
        read) GET_FAIL=1 ;;
        write) SET_FAIL=1 ;;
        marker) printf '%s\n' '# BEGIN remote.futrx container API DNS' 'server=10.8.0.2' > "$STATE" ;;
        address) address=none ;;
        loopback-address) address=127.0.1.1 ;;
        invalid-address) address=10.8.0.999 ;;
        hostname) hostname=$'remote.example.com\nserver=evil' ;;
    esac
    original="$(cat "$STATE")"
    if ensure_container_api_dns lxdbr0 "$hostname" "$address" >"$TEST_DIR/out" 2>&1; then
        fail "accepted $mode failure"
    fi
    [ "$(cat "$STATE")" = "$original" ] || fail "$mode modified config"
done

# Both update modes invoke install.sh even though they skip public DNS checks.
(
    . "$INFRA_DIR/update.sh"
    SCRIPT_INFRA_DIR="$INFRA_DIR"
    HOSTNAME=remote.example.com
    INCLUDE_BUSY=0
    bash() { printf '%s\n' "$*" >> "$CALLS"; }
    FUTRX_UPDATE_PROGRESS_PATH="$TEST_DIR/progress.json"
    export FUTRX_UPDATE_PROGRESS_PATH
    for UPDATE_WORKSPACES in 0 1; do
        : > "$CALLS"
        remote_converge_update >/dev/null
        grep -Fxq "$INFRA_DIR/install.sh remote.example.com --skip-dns-check" "$CALLS" ||
            fail "update mode $UPDATE_WORKSPACES bypasses installer"
    done
)
grep -Fq 'ensure_container_api_dns "$LXD_BRIDGE" "$HOSTNAME" "${LXD_BRIDGE_IP:-}"' \
    "$INFRA_DIR/steps/01-host-deps.sh" || fail "host convergence bypasses DNS setup"

echo "PASS: container-api-dns"
