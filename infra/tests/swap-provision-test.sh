#!/usr/bin/env bash
set -euo pipefail

# shellcheck source=../lib/swap-provision.sh
. "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/lib/swap-provision.sh"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

eq() {
    # eq DESCRIPTION EXPECTED ACTUAL
    [ "$2" = "$3" ] || fail "$1: got '$3', want '$2'"
}

# ───────────────── swap_desired_mib: RAM tiers ─────────────────
eq "1 GiB RAM -> 2x"        2048  "$(swap_desired_mib 1024)"
eq "2 GiB RAM -> 2x"        4096  "$(swap_desired_mib 2048)"
eq "4 GiB RAM -> 1x (the reported fix)" 4096 "$(swap_desired_mib 4096)"
eq "8 GiB RAM -> 1x"        8192  "$(swap_desired_mib 8192)"
eq "16 GiB RAM -> capped"   8192  "$(swap_desired_mib 16384)"
eq "64 GiB RAM -> capped"   8192  "$(swap_desired_mib 65536)"

# ───────────────── swap_reserve_mib: disk headroom ─────────────────
eq "small disk reserves 2 GiB floor" 2048 "$(swap_reserve_mib 8000)"
eq "large disk reserves 20%"         20000 "$(swap_reserve_mib 100000)"

# ───────────────── swap_round_down_mib ─────────────────
eq "rounds down to 512 multiple" 3584 "$(swap_round_down_mib 3800)"
eq "already a multiple"          4096 "$(swap_round_down_mib 4096)"

# ───────────────── swap_plan: the whole decision ─────────────────

# The exact scenario from the issue: 4 GiB RAM, no swap, roomy disk.
# Wants 1x RAM = 4096, disk allows it.
eq "issue box: 4 GiB RAM, no swap, 40 GiB free/50 GiB disk" \
   4096 "$(swap_plan 4096 0 40960 51200)"

# Operator already has meaningful swap -> never stack on top.
eq "existing swap is respected" \
   0 "$(swap_plan 4096 2048 40960 51200)"

# Tiny leftover swap (< SWAP_MIN_MIB) is not "meaningful" -> still provision.
eq "sub-minimum existing swap is ignored" \
   4096 "$(swap_plan 4096 128 40960 51200)"

# Disk-constrained: desired is 4096 but only ~3 GiB free after the 2 GiB
# reserve, rounded down to a 512 multiple.
eq "disk caps the swap size" \
   3072 "$(swap_plan 4096 0 5120 9000)"

# No room at all: free minus reserve is below the minimum -> skip.
eq "too little free disk -> skip" \
   0 "$(swap_plan 4096 0 2200 8000)"

# Fitted size rounds below the minimum -> skip rather than make a token file.
eq "fitted below SWAP_MIN_MIB -> skip" \
   0 "$(swap_plan 4096 0 2300 6000)"

# Tiny box, plenty of disk: 1 GiB RAM wants 2x = 2048.
eq "1 GiB box gets 2x on a roomy disk" \
   2048 "$(swap_plan 1024 0 40960 51200)"

# Big box, capped desired, disk allows it.
eq "32 GiB box capped at max" \
   8192 "$(swap_plan 32768 0 100000 200000)"

# ───────────────── ensure_swap: mocked, no real allocation ─────────────────
# Drive ensure_swap end-to-end with every system touch stubbed, asserting it
# allocates the planned size and wires swapon/mkswap/fstab/sysctl.
ALLOCATED_PATH=""
ALLOCATED_SIZE=""
SWAPON_CALLED=0
MKSWAP_CALLED=0

swap_ram_mib()        { echo 4096; }
swap_active_mib()     { echo 0; }
swap_disk_free_mib()  { echo 40960; }
swap_disk_total_mib() { echo 51200; }
swap_allocate_file()  { ALLOCATED_PATH="$1"; ALLOCATED_SIZE="$2"; }
chmod()               { :; }
mkswap()              { MKSWAP_CALLED=1; }
swapon()              { SWAPON_CALLED=1; }
sysctl()              { :; }
# Point every persistent write at temp paths so the test never edits the host.
TEST_TMP="$(mktemp -d)"
trap 'rm -rf "$TEST_TMP"' EXIT
SWAP_FILE="$TEST_TMP/swapfile"
SWAP_FSTAB="$TEST_TMP/fstab"
SWAP_SWAPPINESS_CONF="$TEST_TMP/swappiness.conf"
: > "$SWAP_FSTAB"

# We assert on observable side effects: the allocated size, that mkswap/swapon
# ran, and that fstab + the sysctl drop-in were written.
ensure_swap >/dev/null 2>&1 || true

eq "ensure_swap allocated at SWAP_FILE" "$SWAP_FILE" "$ALLOCATED_PATH"
eq "ensure_swap allocated planned size" 4096 "$ALLOCATED_SIZE"
eq "ensure_swap ran mkswap" 1 "$MKSWAP_CALLED"
eq "ensure_swap ran swapon" 1 "$SWAPON_CALLED"
grep -qx "$SWAP_FILE none swap sw 0 0" "$SWAP_FSTAB" || fail "ensure_swap did not add an fstab entry"
grep -qx "vm.swappiness=10" "$SWAP_SWAPPINESS_CONF" || fail "ensure_swap did not write the swappiness drop-in"

# When meaningful swap already exists, ensure_swap must not allocate anything.
ALLOCATED_SIZE=""
swap_active_mib() { echo 4096; }
ensure_swap >/dev/null 2>&1 || true
eq "ensure_swap is a no-op when swap exists" "" "$ALLOCATED_SIZE"

echo "swap provisioning tests passed"
