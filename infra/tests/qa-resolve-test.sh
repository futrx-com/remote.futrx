#!/usr/bin/env bash
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=../qa/common.sh
. "$TESTS_DIR/../qa/common.sh"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# Every case stubs the resolver with output captured from the real tool, so
# the suite stays hermetic and runs the same on Linux, macOS, and Git Bash on
# Windows — none of which has the whole set of resolvers installed.

getent() {
    printf '%s\n' \
        '203.0.113.7     STREAM qa.example' \
        '203.0.113.7     DGRAM' \
        '203.0.113.8     RAW'
}
got="$(qa_resolve_ipv4 qa.example getent)"
[ "$got" = "203.0.113.7" ] || fail "getent: expected 203.0.113.7, got '$got'"

dscacheutil() {
    printf '%s\n' \
        'name: qa.example' \
        'ipv6_address: 2001:db8::1' \
        'ip_address: 203.0.113.7' \
        '' \
        'name: qa.example' \
        'ip_address: 203.0.113.8'
}
got="$(qa_resolve_ipv4 qa.example dscacheutil)"
[ "$got" = "203.0.113.7" ] || fail "dscacheutil: expected 203.0.113.7, got '$got'"

# The CNAME line dig prints ahead of the address must not be read as one.
dig() {
    printf '%s\n' 'target.example.' '203.0.113.7' '203.0.113.8'
}
got="$(qa_resolve_ipv4 qa.example dig)"
[ "$got" = "203.0.113.7" ] || fail "dig: expected 203.0.113.7, got '$got'"

host() {
    printf '%s\n' \
        'qa.example is an alias for target.example.' \
        'target.example has address 203.0.113.7' \
        'target.example has IPv6 address 2001:db8::1'
}
got="$(qa_resolve_ipv4 qa.example host)"
[ "$got" = "203.0.113.7" ] || fail "host: expected 203.0.113.7, got '$got'"

# Windows nslookup: the resolver's own address heads the reply and must not be
# mistaken for the answer, and the addresses continue on unlabelled lines.
nslookup() {
    printf '%s\n' \
        'Server:  UnKnown' \
        'Address:  192.168.1.1' \
        '' \
        'Non-authoritative answer:' \
        'Name:    qa.example' \
        'Addresses:  2001:db8::1' \
        '          203.0.113.7' \
        '          203.0.113.8'
}
got="$(qa_resolve_ipv4 qa.example nslookup)"
[ "$got" = "203.0.113.7" ] || fail "windows nslookup: expected 203.0.113.7, got '$got'"

# BIND nslookup, as found on Linux and macOS, prints a different shape.
nslookup() {
    printf '%s\n' \
        'Server:		1.1.1.1' \
        'Address:	1.1.1.1#53' \
        '' \
        'Non-authoritative answer:' \
        'Name:	qa.example' \
        'Address: 203.0.113.7'
}
got="$(qa_resolve_ipv4 qa.example nslookup)"
[ "$got" = "203.0.113.7" ] || fail "bind nslookup: expected 203.0.113.7, got '$got'"

# The DNS-over-HTTPS fallback: curl is the one lookup tool guaranteed to be
# present, since the pre-flight already requires it.
DOH_BODY=""
DOH_EXIT=0
# The call count lives in a file: qa_resolve_ipv4 runs in a command
# substitution, so a counter variable would be lost with its subshell.
CURL_CALL_LOG="$(mktemp)"
curl() {
    printf 'x' >>"$CURL_CALL_LOG"
    [ "$DOH_EXIT" -eq 0 ] || return "$DOH_EXIT"
    printf '%s' "$DOH_BODY"
}
curl_calls() {
    wc -c <"$CURL_CALL_LOG" | tr -d '[:space:]'
}

DOH_BODY='{"Status":0,"Answer":[{"name":"qa.example","type":5,"data":"target.example."},{"name":"target.example","type":1,"data":"203.0.113.7"},{"data":"203.0.113.8"}]}'
got="$(qa_resolve_ipv4 qa.example curl)"
[ "$got" = "203.0.113.7" ] || fail "doh: expected 203.0.113.7, got '$got'"

# NXDOMAIN is a definitive answer: no address, and no second resolver queried.
DOH_BODY='{"Status":3,"Question":[{"name":"nope.example","type":1}]}'
: >"$CURL_CALL_LOG"
got="$(qa_resolve_ipv4 nope.example curl || true)"
[ -z "$got" ] || fail "doh: expected no address for NXDOMAIN, got '$got'"
[ "$(curl_calls)" -eq 1 ] || fail "doh: NXDOMAIN queried $(curl_calls) endpoints, expected 1"

# Both DoH endpoints unreachable is an empty result, not a hang or a crash.
DOH_EXIT=7
: >"$CURL_CALL_LOG"
got="$(qa_resolve_ipv4 qa.example curl || true)"
[ -z "$got" ] || fail "doh: expected no address when unreachable, got '$got'"
[ "$(curl_calls)" -eq 2 ] || fail "doh: tried $(curl_calls) endpoints, expected both"

# A name with no A record is a reportable condition, not a script abort: the
# resolver exits non-zero and the caller still gets an empty string back.
getent() { return 2; }
got="$(qa_resolve_ipv4 nope.example getent || true)"
[ -z "$got" ] || fail "expected no address for an unresolvable name, got '$got'"

# Resolver selection prefers the platform's native tool and keeps DoH last, so
# a machine that already resolves names locally keeps doing so.
unset -f getent dscacheutil dig host nslookup curl
stub_dir="$(mktemp -d)"
trap 'rm -rf -- "$stub_dir" "$CURL_CALL_LOG"' EXIT
resolvers="getent dscacheutil dig host nslookup curl"
for name in $resolvers; do
    printf '#!/bin/sh\nexit 0\n' >"$stub_dir/$name"
    chmod +x "$stub_dir/$name"
done

for expected in $resolvers; do
    got="$(PATH="$stub_dir" qa_ipv4_resolver)"
    [ "$got" = "$expected" ] || fail "expected $expected to win resolver selection, got '$got'"
    rm -f "$stub_dir/$expected"
done

if got="$(PATH="$stub_dir" qa_ipv4_resolver)"; then
    fail "expected no resolver to be reported, got '$got'"
fi

echo "QA resolver tests passed"
