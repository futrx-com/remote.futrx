#!/usr/bin/env bash
# Swap-file provisioning for hosts with little or no swap.
#
# Why this exists: running several dev servers at once (e.g. an Nx workspace
# starting many apps) produces a large, short-lived RSS spike — on the order
# of 5x steady-state for a second or two. Steady state fits in RAM with room
# to spare; only the peak is the problem. On a box with no swap that transient
# peak is unsurvivable: it trips the OOM killer and takes the dev servers down.
# A modest swap file lets the kernel absorb the spike and page it back out
# within a second, instead of killing processes.
#
# The size is not fixed. Some boxes are short on disk, so we size the swap file
# from three inputs — installed RAM, free disk, and total disk — and skip the
# whole thing when there is no meaningful room. Existing operator-configured
# swap is left untouched.
#
# The pure sizing functions (swap_*_mib, swap_plan) take integers and print
# integers so they can be unit-tested without touching the system. The thin
# wrappers below them read /proc/meminfo and df; the action functions create
# the file. See infra/tests/swap-provision-test.sh.

# All sizes are MiB integers.

# A swap file smaller than this is not worth the fstab entry / disk churn.
SWAP_MIN_MIB="${SWAP_MIN_MIB:-512}"
# Never let a swap file grow past this, however much RAM/disk is available —
# these are dev boxes, not databases; the spike we're buffering is a few GB.
SWAP_MAX_MIB="${SWAP_MAX_MIB:-8192}"
# Canonical swap-file path. Overridable so tests never touch a real /swapfile.
SWAP_FILE="${SWAP_FILE:-/swapfile}"
# The persistence targets, likewise overridable so tests stay hermetic.
SWAP_FSTAB="${SWAP_FSTAB:-/etc/fstab}"
SWAP_SWAPPINESS_CONF="${SWAP_SWAPPINESS_CONF:-/etc/sysctl.d/60-futrx-swappiness.conf}"

# swap_desired_mib RAM_MIB
# Ideal swap for a memory-spike workload, before any disk constraint:
#   - tiny boxes (<= 2 GiB) get 2x RAM — the spike dwarfs their RAM
#   - small/medium boxes (<= 8 GiB) get 1x RAM — matches the 4 GiB-on-4 GiB fix
#   - larger boxes are capped at SWAP_MAX_MIB; steady state already fits
swap_desired_mib() {
    local ram_mib="$1" desired
    if [ "$ram_mib" -le 2048 ]; then
        desired="$((ram_mib * 2))"
    elif [ "$ram_mib" -le 8192 ]; then
        desired="$ram_mib"
    else
        desired="$SWAP_MAX_MIB"
    fi
    [ "$desired" -gt "$SWAP_MAX_MIB" ] && desired="$SWAP_MAX_MIB"
    printf '%s\n' "$desired"
}

# swap_reserve_mib TOTAL_DISK_MIB
# Free disk we refuse to hand to swap, so provisioning never fills the disk:
# the larger of 2 GiB or 20% of the filesystem.
swap_reserve_mib() {
    local total_mib="$1" pct
    pct="$((total_mib / 5))"
    if [ "$pct" -gt 2048 ]; then
        printf '%s\n' "$pct"
    else
        printf '%s\n' 2048
    fi
}

# swap_round_down_mib SIZE_MIB
# Round down to a whole 512 MiB so we allocate tidy sizes.
swap_round_down_mib() {
    local size_mib="$1"
    printf '%s\n' "$(( (size_mib / 512) * 512 ))"
}

# swap_plan RAM_MIB EXISTING_SWAP_MIB FREE_DISK_MIB TOTAL_DISK_MIB
# Prints the swap-file size to create in MiB, or 0 to do nothing. 0 means one
# of: meaningful swap already exists, the disk can't spare SWAP_MIN_MIB after
# its reserve, or the fitted size rounds below SWAP_MIN_MIB.
swap_plan() {
    local ram_mib="$1" existing_mib="$2" free_mib="$3" total_mib="$4"
    local desired reserve allowed target

    # Respect an operator who already configured swap — never stack on top.
    if [ "$existing_mib" -ge "$SWAP_MIN_MIB" ]; then
        printf '%s\n' 0
        return 0
    fi

    desired="$(swap_desired_mib "$ram_mib")"
    reserve="$(swap_reserve_mib "$total_mib")"
    allowed="$((free_mib - reserve))"
    if [ "$allowed" -lt "$SWAP_MIN_MIB" ]; then
        printf '%s\n' 0
        return 0
    fi

    target="$desired"
    [ "$target" -gt "$allowed" ] && target="$allowed"
    target="$(swap_round_down_mib "$target")"
    if [ "$target" -lt "$SWAP_MIN_MIB" ]; then
        printf '%s\n' 0
        return 0
    fi
    printf '%s\n' "$target"
}

# ───────────────── system readers (mocked in tests) ─────────────────

# Installed RAM, MiB. MemTotal in /proc/meminfo is kB.
swap_ram_mib() {
    awk '/^MemTotal:/ { printf "%d\n", $2 / 1024; exit }' /proc/meminfo
}

# Currently active swap, MiB. SwapTotal in /proc/meminfo is kB.
swap_active_mib() {
    awk '/^SwapTotal:/ { printf "%d\n", $2 / 1024; exit }' /proc/meminfo
}

# Free / total space, MiB, on the filesystem that will hold SWAP_FILE. We probe
# the parent directory because SWAP_FILE itself may not exist yet.
swap_disk_free_mib() {
    df -P --block-size=1M "$(dirname "$SWAP_FILE")" | awk 'NR==2 { print $4; exit }'
}
swap_disk_total_mib() {
    df -P --block-size=1M "$(dirname "$SWAP_FILE")" | awk 'NR==2 { print $2; exit }'
}

# ───────────────── logging ─────────────────
# install.sh exports styled log/ok/warn helpers; when the lib is sourced
# standalone (tests, manual runs) those don't exist, so fall back to plain
# echo. Defined once at file scope rather than per ensure_swap call.
swap_log()  { if command -v log  >/dev/null 2>&1; then log  "$@"; else echo "==> $*"; fi; }
swap_ok()   { if command -v ok   >/dev/null 2>&1; then ok   "$@"; else echo "OK $*"; fi; }
swap_warn() { if command -v warn >/dev/null 2>&1; then warn "$@"; else echo "!! $*" >&2; fi; }

# ───────────────── actions ─────────────────

# swap_allocate_file PATH SIZE_MIB
# Prefer fallocate (instant); fall back to dd when the filesystem can't hand
# swapon a fallocated file (some setups leave holes swapon rejects).
swap_allocate_file() {
    local path="$1" size_mib="$2"
    rm -f "$path"
    if fallocate -l "${size_mib}M" "$path" 2>/dev/null; then
        return 0
    fi
    rm -f "$path"
    dd if=/dev/zero of="$path" bs=1M count="$size_mib" status=none
}

# swap_persist
# Make an already-active swap file durable and tune the kernel for it. Two
# concerns, both "keep it this way past this boot": an fstab entry so the file
# is re-enabled after a reboot, and a swappiness drop-in so steady state stays
# in RAM and swap is only touched under real pressure (the workload fits in RAM
# with room to spare — swap is a spike buffer, not a place to page warm pages).
swap_persist() {
    if ! grep -qE "^[^#]*[[:space:]]${SWAP_FILE}[[:space:]]|^${SWAP_FILE}[[:space:]]" "$SWAP_FSTAB" 2>/dev/null; then
        printf '%s none swap sw 0 0\n' "$SWAP_FILE" >> "$SWAP_FSTAB"
    fi
    mkdir -p "$(dirname "$SWAP_SWAPPINESS_CONF")"
    printf 'vm.swappiness=10\n' > "$SWAP_SWAPPINESS_CONF"
    sysctl -q vm.swappiness=10 2>/dev/null || true
}

# ensure_swap
# Full orchestration for the installer step. Idempotent: safe on every re-run.
ensure_swap() {
    local ram_mib existing_mib free_mib total_mib target

    ram_mib="$(swap_ram_mib)"
    existing_mib="$(swap_active_mib)"

    if [ "$existing_mib" -ge "$SWAP_MIN_MIB" ]; then
        swap_ok "swap already present (${existing_mib} MiB) — leaving it as is"
        return 0
    fi

    free_mib="$(swap_disk_free_mib)"
    total_mib="$(swap_disk_total_mib)"
    target="$(swap_plan "$ram_mib" "$existing_mib" "$free_mib" "$total_mib")"

    if [ "$target" -eq 0 ]; then
        swap_warn "no swap configured: ${ram_mib} MiB RAM, ${free_mib} MiB free of ${total_mib} MiB disk — not enough room to add swap safely. A large memory spike could OOM-kill dev servers."
        return 0
    fi

    # If our own swap file already exists and is active, we're done. If it
    # exists but isn't on (e.g. a reboot before fstab took effect), turn it on.
    if [ -f "$SWAP_FILE" ] && swapon --show=NAME --noheadings 2>/dev/null | grep -qx "$SWAP_FILE"; then
        swap_ok "swap file $SWAP_FILE already active"
        return 0
    fi

    swap_log "Provisioning ${target} MiB swap at $SWAP_FILE (RAM ${ram_mib} MiB, ${free_mib} MiB free of ${total_mib} MiB disk)"
    swap_allocate_file "$SWAP_FILE" "$target"
    chmod 600 "$SWAP_FILE"
    mkswap "$SWAP_FILE" >/dev/null
    swapon "$SWAP_FILE"
    swap_persist

    swap_ok "swap active: $(swap_active_mib) MiB (vm.swappiness=10)"
}
