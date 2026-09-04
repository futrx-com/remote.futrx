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

INFRA_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
DATA_DIR="${FUTRX_DATA_DIR:-/opt/remote.futrx/data}"
MAINTENANCE_FILE="${FUTRX_MAINTENANCE_FILE:-$DATA_DIR/self-update/maintenance.json}"

log()  { printf "\n\033[1;36m==> %s\033[0m\n" "$*"; }
warn() { printf "\033[1;33m!! %s\033[0m\n" "$*"; }
ok()   { printf "\033[1;32m✓\033[0m %s\n" "$*"; }
err()  { printf "\n\033[1;31m✗ %s\033[0m\n" "$*" >&2; }

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

if [ "$EUID" -ne 0 ]; then
    err "needs root; rerun with sudo"
    exit 1
fi
if ! command -v lxc >/dev/null; then
    err "lxc CLI not found"
    exit 1
fi
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

# ───────────────── 2. migrate + replace through Go ─────────────────
log "Migrating and replacing project containers"
GO_ARGS=()
[ "$DRY_RUN" -eq 0 ] || GO_ARGS+=(--dry-run)
[ "$INCLUDE_BUSY" -eq 0 ] || GO_ARGS+=(--include-busy)
[ -z "${FUTRX_UPDATE_PROGRESS_PATH:-}" ] || GO_ARGS+=(--progress-file "$FUTRX_UPDATE_PROGRESS_PATH")

# Keep the control plane and update-status polling available throughout the
# migration. The backend checks this live-PID marker before accepting a new
# prompt, closing the race that previously required stopping the whole service.
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
