#!/usr/bin/env bash
# Shared output, guard, and validation helpers for the infra entry points
# (install.sh, update.sh, deploy-app.sh, upgrade-workspaces.sh).
#
# Source this file — it executes nothing on its own. Callers are expected to
# run under `set -euo pipefail`. The curl|bash bootstrap block at the top of
# install.sh intentionally stays self-contained and must not source this file
# (no checkout exists on disk yet at that point).

# Colored status output. Steps (infra/steps/*.sh) rely on these via
# `export -f` from the entry point that sources them.
log()  { printf "\n\033[1;36m==> %s\033[0m\n" "$*"; }
warn() { printf "\n\033[1;33m!! %s\033[0m\n" "$*"; }
ok()   { printf "\033[1;32m✓\033[0m %s\n" "$*"; }
err()  { printf "\n\033[1;31m✗ %s\033[0m\n" "$*" >&2; }

# die MESSAGE...
# Print a plain error to stderr and exit 1. Use err() + exit for styled or
# non-1 exits instead.
die() {
    printf '%s\n' "$*" >&2
    exit 1
}

# require_root SUBJECT
# Abort unless running as root, e.g. require_root "this installer" prints
# "this installer needs root; rerun with sudo".
require_root() {
    if [ "$EUID" -ne 0 ]; then
        printf '%s needs root; rerun with sudo\n' "$1" >&2
        exit 1
    fi
}

# validate_full_sha VALUE
# Abort unless VALUE is a full 40-character commit SHA. The error text is
# pinned by infra/tests/qa-scripts-test.sh — keep it byte-identical.
validate_full_sha() {
    if ! printf '%s' "${1:-}" | grep -qE '^[0-9a-fA-F]{40}$'; then
        printf -- '--ref must be a full 40-character commit SHA\n' >&2
        exit 1
    fi
}

# script_dir
# Print the directory containing the calling script. Call at the caller's top
# level so BASH_SOURCE[1] is the caller's file.
script_dir() {
    ( cd "$(dirname "${BASH_SOURCE[1]}")" >/dev/null 2>&1 && pwd )
}
