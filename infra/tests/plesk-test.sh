#!/usr/bin/env bash
set -euo pipefail

TEST_TMP="$(mktemp -d)"
trap 'rm -rf "$TEST_TMP"' EXIT

# Point detection at a sandbox root, so the suite reports the same answer
# whether or not the machine running it happens to have Plesk installed.
export FUTRX_PLESK_ROOTS="$TEST_TMP/psa"

# shellcheck source=../lib/plesk.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib/plesk.sh"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# ───────────────── detection ─────────────────

# No Plesk: the installer must take its original path unchanged. Guard against
# a `plesk` binary on the tester's PATH so this asserts what it claims to.
if ! command -v plesk >/dev/null 2>&1; then
    plesk_present && fail "expected no Plesk on a host without the marker"
fi
plesk_root >/dev/null 2>&1 && fail "expected no Plesk root"

mkdir -p "$TEST_TMP/psa"
printf '18.0.62 Ubuntu 24.04 1815240321.20\n' > "$TEST_TMP/psa/version"

plesk_present || fail "expected Plesk to be detected from the version marker"
[ "$(plesk_root)" = "$TEST_TMP/psa" ] || fail "wrong root: $(plesk_root)"
[ "$(plesk_version)" = "18.0.62" ] || fail "wrong version: $(plesk_version)"

# ───────────────── certificate SAN coverage ─────────────────

if ! command -v openssl >/dev/null 2>&1; then
    echo "SKIP: openssl unavailable, certificate coverage untested" >&2
    echo "PASS: plesk"
    exit 0
fi

make_cert() {
    local out="$1"; shift
    local sans="" name
    for name in "$@"; do
        sans="${sans:+$sans,}DNS:$name"
    done
    openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
        -subj "/CN=$1" -addext "subjectAltName=$sans" \
        -keyout "$out.key" -out "$out" >/dev/null 2>&1
    # Plesk concatenates key and certificate into one file; nginx takes it for
    # both ssl_certificate and ssl_certificate_key, and so must our check.
    cat "$out.key" >> "$out"
    rm -f "$out.key"
}

HOST="remote.example.com"
NEEDED=("$HOST" "code.$HOST" "*.code.$HOST" "*.dev.$HOST")

STORE="$TEST_TMP/psa/var/certificates"
mkdir -p "$STORE"

# The trap: a certificate for the bare hostname only. It makes the main UI load
# and every preview and IDE URL fail with a certificate error, so it must not
# be accepted.
make_cert "$STORE/cert-bare" "$HOST"
cert_covers "$STORE/cert-bare" "${NEEDED[@]}" \
    && fail "a hostname-only certificate must not be accepted for the wildcards"
cert_covers "$STORE/cert-bare" "$HOST" || fail "expected the bare hostname to be covered"
plesk_cert_for "${NEEDED[@]}" >/dev/null 2>&1 \
    && fail "the store holds no usable certificate yet"

# A wildcard SAN covers exactly one label — this is the rule that decides
# whether *.dev.<host> works, so assert both directions.
make_cert "$STORE/cert-wild" "$HOST" "*.$HOST" "*.code.$HOST" "*.dev.$HOST"
cert_covers "$STORE/cert-wild" "${NEEDED[@]}" \
    || fail "expected the wildcard certificate to cover every name"
cert_covers "$STORE/cert-wild" "myproj--3000.dev.$HOST" \
    || fail "*.dev.<host> should cover a single-label preview host"
cert_covers "$STORE/cert-wild" "a.b.dev.$HOST" \
    && fail "*.dev.<host> must not cover a two-label subdomain"
cert_covers "$STORE/cert-wild" "other.example.org" \
    && fail "an unrelated name must not be reported as covered"

[ "$(plesk_cert_for "${NEEDED[@]}")" = "$STORE/cert-wild" ] \
    || fail "expected the wildcard certificate to be selected, got: $(plesk_cert_for "${NEEDED[@]}" || true)"

# An unreadable path is a miss, not a crash.
cert_covers "$TEST_TMP/nope" "$HOST" && fail "a missing certificate must not be accepted"

echo "PASS: plesk"
