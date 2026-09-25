#!/usr/bin/env bash
# upgrade-workspaces.sh — converge every project container to the current
# base image.
#
# Flow:
#   1. Rebake futrx-remote-dev-base from the recipe (skip with --no-rebake,
#      e.g. when you just rebaked by hand).
#   2. Delegate replacement to the Go lifecycle. It migrates agent homes,
#      replaces each idle container, attaches every durable mount, and
#      validates the result before reporting success.
#
# Safety:
#   - Workspace files always survive (bind-mounted from the host). Anything
#     installed in the container ROOTFS outside /workspace (ad-hoc apt/npm
#     installs, caches) is lost — that is what "upgrade by re-clone" means.
#   - Containers with a running agent process are SKIPPED by default.
#   - The control plane stays online in maintenance mode so update progress is
#     visible, while new prompts are rejected until replacement is complete.
#   - Go reads the project repository; unrelated LXD containers are untouched.
#
# Usage:
#   sudo bash /opt/remote.futrx/infra/upgrade-workspaces.sh [flags]
#
# Flags:
#   --dry-run        show what would happen, change nothing
#   --no-rebake      skip step 1, only recycle containers
#   --include-busy   also replace containers with an active agent process
set -euo pipefail

begin_maintenance() {
    local maintenance_dir temporary
    maintenance_dir="$(dirname "$MAINTENANCE_FILE")"
    mkdir -p "$maintenance_dir"
    chmod 700 "$maintenance_dir"
    temporary="${MAINTENANCE_FILE}.tmp.$$"
    printf '{"pid":%d,"startedAt":%d}\n' "$$" "$(date +%s)" > "$temporary"
    chmod 600 "$temporary"
    mv "$temporary" "$MAINTENANCE_FILE"
}
end_maintenance() {
    rm -f "$MAINTENANCE_FILE"
}
remote_load_configuration() {
INFRA_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
# shellcheck source=lib/common.sh
. "$INFRA_DIR/lib/common.sh"
DATA_DIR="${FUTRX_DATA_DIR:-/opt/remote.futrx/data}"
MAINTENANCE_FILE="${FUTRX_MAINTENANCE_FILE:-$DATA_DIR/self-update/maintenance.json}"
}

remote_parse_workspace_arguments() {
DRY_RUN=0
REBAKE=1
INCLUDE_BUSY=0
for a in "$@"; do
    case "$a" in
        --dry-run)      DRY_RUN=1 ;;
        --no-rebake)    REBAKE=0 ;;
        --include-busy) INCLUDE_BUSY=1 ;;
        *) err "unknown flag: $a"; exit 1 ;;
    esac
done
}

remote_validate_host() {
# ───────────────── 0. validate ─────────────────
if ! command -v lxc >/dev/null; then
    err "lxc CLI not found"
    exit 1
fi
}

remote_rebake_base_image() {
# ───────────────── 1. rebake ─────────────────
if [ "$REBAKE" -eq 1 ]; then
    if [ "$DRY_RUN" -eq 1 ]; then
        log "[dry-run] would rebake base image (go run ./cmd/build-base-image -overwrite)"
    else
        log "Rebaking base image (60-120s)"
        ( cd "$INFRA_DIR/../backend" && go run ./cmd/build-base-image -overwrite )
        ok "base image rebaked"
    fi
else
    log "Skipping rebake (--no-rebake) — recycling containers onto the existing image"
fi
}

remote_migrate_containers() {
# ───────────────── 2. migrate + replace through Go ─────────────────
log "Migrating and replacing project containers"
GO_ARGS=()
[ "$DRY_RUN" -eq 0 ] || GO_ARGS+=(--dry-run)
[ "$INCLUDE_BUSY" -eq 0 ] || GO_ARGS+=(--include-busy)
[ -z "${FUTRX_UPDATE_PROGRESS_PATH:-}" ] || GO_ARGS+=(--progress-file "$FUTRX_UPDATE_PROGRESS_PATH")

# Keep the control plane and update-status polling available throughout the
# migration. The backend checks this live-PID marker before accepting a new
# prompt, closing the race that previously required stopping the whole service.
if [ "$DRY_RUN" -eq 0 ]; then
    begin_maintenance
    trap end_maintenance EXIT
fi
(
    cd "$INFRA_DIR/../backend"
    DATA_DIR="$DATA_DIR" go run ./cmd/upgrade-workspaces "${GO_ARGS[@]}"
)
if [ "$DRY_RUN" -eq 0 ]; then
    end_maintenance
    trap - EXIT
fi
ok "workspace lifecycle convergence complete"
}

main() {
    remote_load_configuration
    remote_parse_workspace_arguments "$@"
    require_root "upgrade-workspaces"
    remote_validate_host
    remote_rebake_base_image
    remote_migrate_containers
}

# Sourced (e.g. by tests) - definitions only. Note the guard
# defaults to *executing*: BASH_SOURCE is unset when bash reads
# from stdin (`bash -s`), which must still run (curl|bash mode).
if [[ -n "${BASH_SOURCE[0]:-}" ]] && [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
    return 0 2>/dev/null || exit 0
fi
main "$@"
