#!/usr/bin/env bash
set -euo pipefail
TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$TESTS_DIR/../lib/lxd-storage.sh"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEST_DIR"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }
log() { :; }
warn() { echo "$*" >> "$TEST_DIR/warnings"; }
err() { echo "$*" >> "$TEST_DIR/errors"; }
lxc() {
    case "$*" in
        'storage list --format=json') printf '%s\n' "$TEST_POOLS" ;;
        'query /1.0/profiles/default') printf '%s\n' "$TEST_PROFILE" ;;
        'query /1.0/storage-pools/existing') printf '%s\n' "$TEST_EXISTING" ;;
        'query /1.0') printf '%s\n' "$TEST_INFO" ;;
        'network show lxdbr0') [ "$TEST_BRIDGE" = yes ] ;;
        *) fail "unexpected LXC command: $*" ;;
    esac
}
lxd() { [ "$*" = 'init --preseed' ] || fail "unexpected LXD command: $*"; cat > "$TEST_DIR/preseed"; return "${TEST_INIT_STATUS:-0}"; }
reset() {
    unset NESTED_UNPRIVILEGED_LXC
    unset FUTRX_STORAGE_DRIVER FUTRX_STORAGE_POOL FUTRX_STORAGE_SOURCE FUTRX_STORAGE_SIZE
    TEST_POOLS='[]'
    TEST_PROFILE='{"devices":{},"config":{}}'
    TEST_INFO='{"environment":{"storage_supported_drivers":[{"Name":"zfs"},{"Name":"btrfs"},{"Name":"dir"}]}}'
    TEST_EXISTING='{"driver":"zfs","config":{"source":"tank/remote","size":"100GiB"}}'
    TEST_BRIDGE=no
    TEST_INIT_STATUS=0
    rm -f "$TEST_DIR/preseed" "$TEST_DIR/warnings" "$TEST_DIR/errors"
}
assert_json() { jq -e "$1" "$TEST_DIR/preseed" >/dev/null || fail "preseed did not satisfy: $1"; }
assert_refused() { if configure_lxd_storage; then fail "accepted invalid setup"; fi; [ ! -e "$TEST_DIR/preseed" ] || fail "mutated refused setup"; }

reset
configure_lxd_storage
assert_json '.storage_pools[0].driver == "zfs" and .profiles[0].devices.root.pool == "default"'
assert_json '.networks[0].config["ipv4.address"] == "auto" and .profiles[0].devices.eth0.network == "lxdbr0"'

reset
TEST_INFO='{"environment":{"storage_supported_drivers":[{"Name":"btrfs"},{"Name":"dir"}]}}'
configure_lxd_storage
assert_json '.storage_pools[0].driver == "btrfs" and .storage_pools[0].config["btrfs.mount_options"] == "user_subvol_rm_allowed,compress=zstd"'

reset
TEST_INFO='{"environment":{"storage_supported_drivers":[{"Name":"dir"}]}}'
configure_lxd_storage
assert_json '.storage_pools[0].driver == "dir"'
[ -s "$TEST_DIR/warnings" ] || fail "silent dir fallback"

reset
FUTRX_STORAGE_DRIVER=zfs FUTRX_STORAGE_SOURCE='tank/remote' FUTRX_STORAGE_POOL=workspaces configure_lxd_storage
assert_json '.storage_pools[0].config.source == "tank/remote" and .storage_pools[0].name == "workspaces" and .profiles[0].devices.root.pool == "workspaces"'

reset
FUTRX_STORAGE_DRIVER=btrfs FUTRX_STORAGE_SIZE=100GiB configure_lxd_storage
assert_json '.storage_pools[0].config.size == "100GiB" and (.storage_pools[0].config | has("source") | not)'

for scenario in invalid_driver unavailable source_auto source_and_size invalid_size dir_size invalid_pool customized bridge; do
    reset
    case "$scenario" in
        invalid_driver) FUTRX_STORAGE_DRIVER=typo ;;
        unavailable) FUTRX_STORAGE_DRIVER=zfs; TEST_INFO='{}' ;;
        source_auto) FUTRX_STORAGE_SOURCE=/dev/example ;;
        source_and_size) FUTRX_STORAGE_DRIVER=zfs; FUTRX_STORAGE_SOURCE=tank/remote; FUTRX_STORAGE_SIZE=100GiB ;;
        invalid_size) FUTRX_STORAGE_SIZE='-1GiB' ;;
        dir_size) FUTRX_STORAGE_DRIVER=dir; FUTRX_STORAGE_SIZE=100GiB ;;
        invalid_pool) FUTRX_STORAGE_POOL='../pool' ;;
        customized) TEST_PROFILE='{"devices":{"disk":{"type":"disk"}}}' ;;
        bridge) TEST_BRIDGE=yes ;;
    esac
    assert_refused
done

reset
TEST_POOLS='[{"name":"existing"}]'
TEST_PROFILE='{"devices":{"root":{"type":"disk","path":"/","pool":"existing"}}}'
TEST_BRIDGE=yes
configure_lxd_storage
[ ! -e "$TEST_DIR/preseed" ] || fail "reinitialized existing storage"
FUTRX_STORAGE_DRIVER=zfs FUTRX_STORAGE_SOURCE=tank/remote FUTRX_STORAGE_POOL=existing FUTRX_STORAGE_SIZE='' configure_lxd_storage
[ ! -e "$TEST_DIR/preseed" ] || fail "reinitialized matching existing storage"
FUTRX_STORAGE_DRIVER=btrfs assert_refused
FUTRX_STORAGE_POOL=other assert_refused
FUTRX_STORAGE_SOURCE=tank/other FUTRX_STORAGE_DRIVER=zfs assert_refused
FUTRX_STORAGE_SIZE=200GiB assert_refused
TEST_BRIDGE=no assert_refused

reset
TEST_INIT_STATUS=1
if configure_lxd_storage; then fail "hid initialization failure"; fi

reset
NESTED_UNPRIVILEGED_LXC=1 configure_lxd_storage
assert_json '.storage_pools[0].driver == "dir"'

# Exercise the real installer argument parser without executing installation.
(
    . "$TESTS_DIR/../install.sh"
    HOSTNAME='' TARGET_REF=''
    remote_parse_install_arguments remote.example.com --storage-driver=btrfs \
        --storage-pool=workspaces --storage-size=100GiB
    [ "$FUTRX_STORAGE_DRIVER" = btrfs ] && [ "$FUTRX_STORAGE_POOL" = workspaces ] &&
        [ "$FUTRX_STORAGE_SIZE" = 100GiB ] || fail 'storage flags not parsed'
)
echo 'LXD storage selection tests passed'
