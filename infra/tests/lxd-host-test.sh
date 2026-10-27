#!/usr/bin/env bash
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=../lib/lxd-host.sh
. "$TESTS_DIR/../lib/lxd-host.sh"

TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEST_DIR"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

systemd-detect-virt() {
    printf '%s\n' "${TEST_VIRT_TYPE:-none}"
}

printf '%s\n' '         0     100000      65536' > "$TEST_DIR/unprivileged.map"
printf '%s\n' '         0          0 4294967295' > "$TEST_DIR/privileged.map"
printf '%s\n' \
    '         0     100000      65536' \
    '   1000000    1100000      65536' > "$TEST_DIR/nested.map"

TEST_VIRT_TYPE=lxc
unprivileged_lxc_host "$TEST_DIR/unprivileged.map" || \
    fail "unprivileged LXC was not detected"
if unprivileged_lxc_host "$TEST_DIR/privileged.map"; then
    fail "privileged LXC was detected as unprivileged"
fi
TEST_VIRT_TYPE=kvm
if unprivileged_lxc_host "$TEST_DIR/unprivileged.map"; then
    fail "a non-LXC virtual machine was detected as LXC"
fi

nested_lxd_idmap_available "$TEST_DIR/nested.map" "$TEST_DIR/nested.map" || \
    fail "delegated nested LXD range was not detected"
if nested_lxd_idmap_available "$TEST_DIR/unprivileged.map" "$TEST_DIR/unprivileged.map"; then
    fail "missing nested LXD range was accepted"
fi

echo "LXD host environment tests passed"
