#!/usr/bin/env bash
# APT refreshes stay strict. A usable installed Caddy makes its Cloudsmith
# repository optional only when that repository refuses downloads with 402.
# Recovery uses temporary source files; the host's sources and trust settings
# are never edited. Other repository failures still stop convergence.

HOST_APT_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

host_apt_source_view() {
    local destination="$1" source_list='' source_parts='' source_file source_paths
    # apt-config emits shell-quoted assignments for the effective source paths,
    # including hosts that override the usual /etc/apt locations.
    source_paths="$(apt-config shell source_list Dir::Etc::sourcelist/f source_parts Dir::Etc::sourceparts/d)" || return 1
    eval "$source_paths" || return 1
    mkdir -p "$destination/sources.list.d" || return 1
    : > "$destination/sources.list" || return 1
    : > "$destination/removed" || return 1
    if [ -f "$source_list" ]; then
        awk -v format=list -v removed_file="$destination/removed" \
            -f "$HOST_APT_LIB_DIR/filter-caddy-apt-sources.awk" "$source_list" \
            > "$destination/sources.list" || return 1
    fi
    if [ -d "$source_parts" ]; then
        for source_file in "$source_parts"/*.list "$source_parts"/*.sources; do
            [ -f "$source_file" ] || continue
            awk -v format="${source_file##*.}" -v removed_file="$destination/removed" \
                -f "$HOST_APT_LIB_DIR/filter-caddy-apt-sources.awk" "$source_file" \
                > "$destination/sources.list.d/${source_file##*/}" || return 1
        done
    fi
    [ -s "$destination/removed" ]
}

host_apt_get() (
    set -euo pipefail
    local apt_binary="${FUTRX_HOST_APT_GET_BIN:-/usr/bin/apt-get}" argument is_update=0
    for argument in "$@"; do
        case "$argument" in
            update) is_update=1; break ;;
            install|remove|upgrade|full-upgrade|dist-upgrade|autoremove|clean|autoclean) break ;;
        esac
    done
    if [ "$is_update" -eq 0 ]; then
        "$apt_binary" "$@"
        return
    fi

    local work_dir status
    work_dir="$(mktemp -d)" || return 1
    trap 'rm -rf -- "$work_dir"' EXIT
    if LC_ALL=C "$apt_binary" -o APT::Update::Error-Mode=any "$@" >"$work_dir/update.log" 2>&1; then
        cat "$work_dir/update.log"
        return
    else
        status=$?
    fi
    cat "$work_dir/update.log" >&2

    # Signature errors, authentication failures, and outages of other sources
    # do not qualify. The misleading "not signed" message accompanying a 402
    # disappears only after a fresh, strictly checked refresh of the other sources.
    if ! grep -Eq '^[EW]: Failed to fetch https?://dl[.]cloudsmith[.]io/public/caddy/stable/deb/debian/[^[:space:]]+[[:space:]]+402[[:space:]]' \
        "$work_dir/update.log" || ! command -v caddy >/dev/null 2>&1 || \
        ! caddy version >/dev/null 2>&1; then
        return "$status"
    fi
    if grep -Eq 'GPG error:|BADSIG|EXPKEYSIG|REVKEYSIG|NO_PUBKEY|OpenPGP signature verification failed|Clearsigned file isn.t valid' "$work_dir/update.log"; then
        return "$status"
    fi
    if ! host_apt_source_view "$work_dir/view"; then
        echo 'Could not isolate the unavailable Caddy repository; stopping the update.' >&2
        return "$status"
    fi
    echo 'Caddy is already installed; Cloudsmith returned HTTP 402. Retrying APT with a temporary source view that omits only the Caddy repository. Host source files are unchanged.' >&2
    LC_ALL=C "$apt_binary" \
        -o APT::Update::Error-Mode=any \
        -o "Dir::Etc::sourcelist=$work_dir/view/sources.list" \
        -o "Dir::Etc::sourceparts=$work_dir/view/sources.list.d" \
        -o APT::Get::List-Cleanup=false \
        "$@"
)

install_nodesource_node() (
    set -euo pipefail
    local node_major="$1" work_dir
    work_dir="$(mktemp -d)" || return 1
    trap 'rm -rf -- "$work_dir"' EXIT
    curl -fsSL "https://deb.nodesource.com/setup_${node_major}.x" > "$work_dir/setup.sh" || return 1
    # NodeSource performs its own APT refreshes and writes new source files.
    # Route those refreshes through the same helper, rebuilding the temporary
    # view each time so its newly added repository remains available.
    printf '#!/usr/bin/env bash\nexec bash %q "$@"\n' \
        "$HOST_APT_LIB_DIR/host-apt.sh" > "$work_dir/apt-get" || return 1
    chmod 0755 "$work_dir/apt-get" || return 1
    PATH="$work_dir:$PATH" bash "$work_dir/setup.sh" >/dev/null || return 1
    host_apt_get install -y -qq nodejs
)

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
    host_apt_get "$@"
fi
