#!/usr/bin/env bash
# Initialize only a fresh LXD installation. Existing pools are never reformatted
# or migrated by an installer rerun. JSON is valid YAML for lxd --preseed.

configure_lxd_storage() {
    local requested="${FUTRX_STORAGE_DRIVER:-auto}"
    local pool="${FUTRX_STORAGE_POOL:-default}"
    local source="${FUTRX_STORAGE_SOURCE:-}"
    local size="${FUTRX_STORAGE_SIZE:-}"
    case "$requested" in auto|dir|zfs|btrfs) ;; *)
        err "Storage driver must be auto, dir, zfs or btrfs."; return 1 ;;
    esac
    if ! [[ "$pool" =~ ^[a-zA-Z0-9][a-zA-Z0-9_-]*$ ]]; then
        err "Invalid LXD storage pool name."; return 1
    fi
    if [ -n "$source" ] && [ "$requested" = auto ]; then
        err "Select a storage driver explicitly when supplying a storage source."; return 1
    fi
    if [ -n "$source" ] && [ -n "$size" ]; then
        err "Use a storage source or a loop pool size, not both."; return 1
    fi
    if [ -n "$size" ] && ! [[ "$size" =~ ^[1-9][0-9]*(GiB|GB|TiB|TB)$ ]]; then
        err "Storage size must be a positive integer with GiB, GB, TiB or TB suffix."; return 1
    fi
    if [ "$requested" = dir ] && [ -n "$size" ]; then
        err "The dir driver does not support a loop pool size."; return 1
    fi

    local pools profile existing_pool existing driver info preseed
    pools="$(lxc storage list --format=json)" || return 1
    profile="$(lxc query /1.0/profiles/default)" || return 1
    if [ "$(jq 'length' <<< "$pools")" -gt 0 ]; then
        existing_pool="$(jq -r '[.devices[]? | select(.type == "disk" and .path == "/") | .pool][0] // empty' <<< "$profile")"
        if [ -z "$existing_pool" ]; then
            err "Existing LXD pools have no default-profile root disk; configure it before installing Remote."; return 1
        fi
        existing="$(lxc query "/1.0/storage-pools/$existing_pool")" || return 1
        driver="$(jq -r '.driver' <<< "$existing")"
        if { [ "$requested" != auto ] && [ "$requested" != "$driver" ]; } ||
           { [ -n "${FUTRX_STORAGE_POOL:-}" ] && [ "$pool" != "$existing_pool" ]; } ||
           { [ -n "$source" ] && [ "$source" != "$(jq -r '.config.source // empty' <<< "$existing")" ]; } ||
           { [ -n "$size" ] && [ "$size" != "$(jq -r '.config.size // empty' <<< "$existing")" ]; }; then
            err "Existing LXD storage differs from the requested configuration. Migrate or resize it explicitly; the installer will not change it."; return 1
        fi
        if ! lxc network show lxdbr0 >/dev/null 2>&1; then
            err "Existing LXD storage has no lxdbr0 bridge; configure networking before installing Remote."; return 1
        fi
        log "Reusing LXD pool $existing_pool ($driver); storage configuration unchanged"
        return 0
    fi

    # Do not overwrite custom profile devices or settings on a partially
    # configured daemon. An operator can finish that setup and rerun.
    if ! jq -e '((.devices // {}) | length) == 0 and ((.config // {}) | length) == 0' <<< "$profile" >/dev/null; then
        err "LXD default profile is already customized; finish storage setup before installing Remote."; return 1
    fi
    info="$(lxc query /1.0)" || return 1
    driver="$requested"
    if [ "$driver" = auto ]; then
        driver=dir
        local candidate
        for candidate in zfs btrfs; do
            # Kernel/tool detection alone does not establish that a nested
            # unprivileged daemon can create a pool. Require explicit setup.
            [ "${NESTED_UNPRIVILEGED_LXC:-0}" != 1 ] || break
            if jq -e --arg name "$candidate" 'any(.environment.storage_supported_drivers[]?; .Name == $name)' <<< "$info" >/dev/null; then
                driver="$candidate"; break
            fi
        done
        if [ "$driver" = dir ]; then
            warn "No automatically usable ZFS/Btrfs driver detected; using dir without copy-on-write or root-disk quotas."
        fi
    elif ! jq -e --arg name "$driver" 'any(.environment.storage_supported_drivers[]?; .Name == $name)' <<< "$info" >/dev/null; then
        err "LXD does not report the requested storage driver as available; install its tools/kernel support first."; return 1
    fi
    if [ "$driver" = dir ] && [ -n "$size" ]; then
        err "No copy-on-write driver is available for the requested loop pool size."; return 1
    fi
    # Refuse to reuse a preexisting bridge implicitly: preseed updates existing
    # resources as well as creating new ones.
    if lxc network show lxdbr0 >/dev/null 2>&1; then
        err "lxdbr0 exists without a storage pool; finish LXD setup before installing Remote."; return 1
    fi
    preseed="$(jq -n --arg driver "$driver" --arg pool "$pool" --arg source "$source" --arg size "$size" '{
        storage_pools: [{name: $pool, driver: $driver, config: (
            (if $source != "" then {source: $source} else {} end) +
            (if $size != "" then {size: $size} else {} end) +
            (if $driver == "btrfs" then {"btrfs.mount_options": "user_subvol_rm_allowed,compress=zstd"} else {} end)
        )}],
        networks: [{name: "lxdbr0", type: "bridge", config: {"ipv4.address": "auto", "ipv6.address": "auto"}}],
        profiles: [{name: "default", devices: {
            root: {type: "disk", path: "/", pool: $pool},
            eth0: {type: "nic", network: "lxdbr0", name: "eth0"}
        }}]
    }')" || return 1
    log "Initializing LXD with $driver storage in pool $pool"
    # No wipe option is supplied, and no device is ever auto-selected. ZFS
    # compression is enabled by LXD's pool creation defaults.
    printf '%s\n' "$preseed" | lxd init --preseed
}
