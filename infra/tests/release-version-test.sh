#!/usr/bin/env bash
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=../lib/release-version.sh
. "$TESTS_DIR/../lib/release-version.sh"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

assert_kind() {
    local current="$1" target="$2" want="$3" got
    got="$(release_update_kind "$current" "$target")"
    [ "$got" = "$want" ] || fail "$current -> $target = $got, want $want"
}

assert_kind 0.3.1 0.3.2 application
assert_kind 0.3.1 0.3.1.1 application
assert_kind v0.3.1 v0.3.2 application
assert_kind 0.3.1 0.4.0 infrastructure
assert_kind 0.3.1 0.4.2 infrastructure
assert_kind 0.4.0 0.4.2 application
assert_kind 1.9.5 2.0.0 infrastructure
assert_kind 0.3 0.3.1 infrastructure
assert_kind dev 0.3.2 infrastructure
assert_kind 0.3.1 candidate infrastructure

echo "Release version tests passed"
