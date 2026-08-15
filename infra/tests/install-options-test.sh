#!/usr/bin/env bash
set -euo pipefail

TEST_TMP="$(mktemp -d)"
trap 'rm -rf "$TEST_TMP"' EXIT

export FUTRX_OPTIONS_FILE="$TEST_TMP/etc/remote.futrx/install-options.env"

# shellcheck source=../lib/install-options.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib/install-options.sh"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# Nothing saved yet: the caller falls through to its own defaults.
load_install_options
[ -z "$FUTRX_SAVED_NO_PLESK_INTEGRATION" ] || fail "expected no saved plesk opt-out"
[ -z "$FUTRX_SAVED_CADDY_HTTP_PORT" ] || fail "expected no saved caddy port"
[ -z "$FUTRX_SAVED_FORCE_SSH_HARDENING" ] || fail "expected no saved ssh choice"

# A missing parent directory is the normal first-install case.
save_install_options 1 8081 1
[ -r "$FUTRX_OPTIONS_FILE" ] || fail "options file was not created"

load_install_options
[ "$FUTRX_SAVED_NO_PLESK_INTEGRATION" = "1" ] || fail "plesk opt-out did not round-trip"
[ "$FUTRX_SAVED_CADDY_HTTP_PORT" = "8081" ] || fail "caddy port did not round-trip"
[ "$FUTRX_SAVED_FORCE_SSH_HARDENING" = "1" ] || fail "ssh choice did not round-trip"

# This is the regression that matters: infra/update.sh re-runs install.sh with
# only the hostname. Without the round-trip above, an operator who chose
# --no-plesk-integration would find Caddy moved behind Plesk on their next
# update, and one who chose --caddy-port=8081 would get nginx pointed at a
# port nothing is listening on.
NO_PLESK_INTEGRATION=""
CADDY_HTTP_PORT=""
NO_PLESK_INTEGRATION="${NO_PLESK_INTEGRATION:-${FUTRX_SAVED_NO_PLESK_INTEGRATION:-0}}"
CADDY_HTTP_PORT="${CADDY_HTTP_PORT:-${FUTRX_SAVED_CADDY_HTTP_PORT:-8080}}"
[ "$NO_PLESK_INTEGRATION" = "1" ] || fail "an update must inherit the saved plesk choice"
[ "$CADDY_HTTP_PORT" = "8081" ] || fail "an update must inherit the saved caddy port"

# An explicit flag still wins over what was remembered.
NO_PLESK_INTEGRATION="0"
NO_PLESK_INTEGRATION="${NO_PLESK_INTEGRATION:-${FUTRX_SAVED_NO_PLESK_INTEGRATION:-0}}"
[ "$NO_PLESK_INTEGRATION" = "0" ] || fail "an explicit flag must override the saved value"

# Overwriting replaces rather than appends, or the file would grow a second
# value for every key and load_install_options would take the last one.
save_install_options 0 8080 0
[ "$(grep -c 'CADDY_HTTP_PORT=' "$FUTRX_OPTIONS_FILE")" -eq 1 ] \
    || fail "saving twice duplicated the keys"
load_install_options
[ "$FUTRX_SAVED_CADDY_HTTP_PORT" = "8080" ] || fail "second save did not take"

# An unwritable location must not fail the install — remembering is a
# convenience, not a requirement.
FUTRX_OPTIONS_FILE="/proc/futrx-cannot-write/options.env"
save_install_options 1 9999 1 || fail "save must never be fatal"
load_install_options || fail "load must never be fatal"

echo "PASS: install-options"
