#!/usr/bin/env bash
# Host-environment checks for installing LXD and preserving Remote's
# unprivileged workspace-container boundary.

unprivileged_lxc_host() {
    local uid_map_file="${1:-/proc/self/uid_map}"
    local virt_type=""

    virt_type="$(systemd-detect-virt --container 2>/dev/null || true)"
    [ "$virt_type" = "lxc" ] || return 1

    # In an unprivileged outer LXC, container UID 0 maps to a non-zero UID
    # on the Proxmox host. A privileged outer container maps 0 to 0.
    awk '$1 == 0 { found = 1; unprivileged = ($2 != 0) }
         END { exit !(found && unprivileged) }' "$uid_map_file"
}

id_range_is_mapped() {
    local map_file="$1" wanted_start="$2" wanted_count="$3"

    awk -v wanted_start="$wanted_start" -v wanted_count="$wanted_count" '
        $1 <= wanted_start && ($1 + $3) >= (wanted_start + wanted_count) {
            found = 1
        }
        END { exit !found }
    ' "$map_file"
}

nested_lxd_idmap_available() {
    local uid_map_file="${1:-/proc/self/uid_map}"
    local gid_map_file="${2:-/proc/self/gid_map}"
    local idmap_base="${3:-1000000}"
    local idmap_size="${4:-65536}"

    id_range_is_mapped "$uid_map_file" "$idmap_base" "$idmap_size" \
        && id_range_is_mapped "$gid_map_file" "$idmap_base" "$idmap_size"
}
