#!/usr/bin/env bash
set -euo pipefail

INFRA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEST_DIR"' EXIT
mkdir -p "$TEST_DIR/bin"
export TEST_DIR

fail() { echo "FAIL: $*" >&2; exit 1; }
warn() { printf '%s\n' "$*" >> "$TEST_DIR/warnings"; }
cat > "$TEST_DIR/bin/loginctl" <<'SCRIPT'
#!/bin/bash
printf '%s\n' "$*" >> "$TEST_DIR/calls"
exit "${LOGINCTL_STATUS:-0}"
SCRIPT
chmod +x "$TEST_DIR/bin/loginctl"

# Sourcing the installer step must not mutate host login settings.
. "$INFRA_DIR/steps/04-backend-svc.sh"
[ ! -e "$TEST_DIR/calls" ] || fail "sourcing the step invoked loginctl"

# Initial install and a repeated update both request the same idempotent action.
PATH="$TEST_DIR/bin" ensure_root_linger
PATH="$TEST_DIR/bin" ensure_root_linger
printf 'enable-linger root\nenable-linger root\n' > "$TEST_DIR/expected"
cmp "$TEST_DIR/expected" "$TEST_DIR/calls" || fail "incorrect loginctl arguments"
[ ! -e "$TEST_DIR/warnings" ] || fail "successful enable emitted a warning"

# A failed or unavailable loginctl must warn without aborting installation.
LOGINCTL_STATUS=1 PATH="$TEST_DIR/bin" ensure_root_linger
grep -q 'Could not enable root linger' "$TEST_DIR/warnings" || fail "failure was silent"
rm "$TEST_DIR/bin/loginctl" "$TEST_DIR/warnings"
PATH="$TEST_DIR/bin" ensure_root_linger
grep -q 'loginctl is unavailable' "$TEST_DIR/warnings" || fail "missing loginctl was silent"

echo "root linger tests passed"
