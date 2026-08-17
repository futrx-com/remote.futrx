#!/usr/bin/env bash
# upgrade-workspaces.sh — converge every project container to the current
# base image.
#
# Flow:
#   1. Rebake futrx-remote-dev-base from the recipe (skip with --no-rebake,
#      e.g. when you just rebaked by hand).
#   1b. Optionally rebake every published project-template image
#      (futrx-remote-<template>-base) on top of the fresh base. Off by
#      default because each one costs another full build.
#   2. Delegate replacement to the Go lifecycle. It migrates agent homes,
#      replaces each idle container, attaches every durable mount, and
#      validates the result before reporting success.
#
# Safety:
#   - Workspace files always survive (bind-mounted from the host). Anything
#     installed in the container ROOTFS outside /workspace (ad-hoc apt/npm
#     installs, caches) is lost — that is what "upgrade by re-clone" means.
#   - Containers with a running agent process are SKIPPED by default.
#   - Go reads the project repository; unrelated LXD containers are untouched.
#
# Usage:
#   sudo bash /opt/remote.futrx/infra/upgrade-workspaces.sh [flags]
#
# Flags:
#   --dry-run           show what would happen, change nothing
#   --no-rebake         skip step 1, only recycle containers
#   --rebake-templates  also rebuild every published template image
#   --include-busy      also replace containers with an active agent process
set -euo pipefail

INFRA_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
DATA_DIR="${FUTRX_DATA_DIR:-/opt/remote.futrx/data}"
UNIT="remote.futrx.service"

log()  { printf "\n\033[1;36m==> %s\033[0m\n" "$*"; }
warn() { printf "\033[1;33m!! %s\033[0m\n" "$*"; }
ok()   { printf "\033[1;32m✓\033[0m %s\n" "$*"; }
err()  { printf "\n\033[1;31m✗ %s\033[0m\n" "$*" >&2; }

DRY_RUN=0
REBAKE=1
REBAKE_TEMPLATES=0
INCLUDE_BUSY=0
for a in "$@"; do
    case "$a" in
        --dry-run)           DRY_RUN=1 ;;
        --no-rebake)         REBAKE=0 ;;
        --rebake-templates)  REBAKE_TEMPLATES=1 ;;
        --include-busy)      INCLUDE_BUSY=1 ;;
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

# ───────── 1b. project-template images ─────────
# Projects created from a template that has a published image launch from that
# image, NOT from futrx-remote-dev-base. Rebaking the base alone therefore
# leaves them on the old base layer.
TEMPLATE_ALIASES="$(lxc image list --format csv --columns l 2>/dev/null \
    | tr ',' '\n' \
    | grep -E '^futrx-remote-.+-base$' \
    | grep -v '^futrx-remote-dev-base$' \
    | sort -u || true)"
if [ -n "$TEMPLATE_ALIASES" ]; then
    if [ "$REBAKE_TEMPLATES" -eq 1 ] && [ "$REBAKE" -eq 1 ]; then
        log "Rebaking published project-template images"
        for ALIAS in $TEMPLATE_ALIASES; do
            TEMPLATE="${ALIAS#futrx-remote-}"
            TEMPLATE="${TEMPLATE%-base}"
            if [ "$DRY_RUN" -eq 1 ]; then
                log "[dry-run] would rebuild $ALIAS (template $TEMPLATE)"
                continue
            fi
            (
                cd "$INFRA_DIR/../backend"
                go run ./cmd/build-template-image -template "$TEMPLATE" -overwrite
            )
            ok "$ALIAS rebaked"
        done
    else
        warn "published template images are NOT rebaked: $(echo $TEMPLATE_ALIASES)"
        warn "projects on those templates keep the old base layer until you rerun with --rebake-templates"
    fi
fi

# ───────────────── 2. migrate + replace through Go ─────────────────
log "Migrating and replacing project containers"
GO_ARGS=()
[ "$DRY_RUN" -eq 0 ] || GO_ARGS+=(--dry-run)
[ "$INCLUDE_BUSY" -eq 0 ] || GO_ARGS+=(--include-busy)

# Prevent a new prompt from racing the standalone upgrade process. The unit's
# KillMode=process leaves any already-running lxc exec child alive; Go sees it
# as busy and skips it unless --include-busy was explicitly requested.
SERVICE_WAS_ACTIVE=0
restart_service() {
    if [ "$SERVICE_WAS_ACTIVE" -eq 1 ]; then
        systemctl start "$UNIT"
    fi
}
if [ "$DRY_RUN" -eq 0 ] && systemctl is-active --quiet "$UNIT"; then
    SERVICE_WAS_ACTIVE=1
    trap restart_service EXIT
    systemctl stop "$UNIT"
fi
(
    cd "$INFRA_DIR/../backend"
    DATA_DIR="$DATA_DIR" go run ./cmd/upgrade-workspaces "${GO_ARGS[@]}"
)
if [ "$SERVICE_WAS_ACTIVE" -eq 1 ]; then
    systemctl start "$UNIT"
    SERVICE_WAS_ACTIVE=0
    trap - EXIT
fi
ok "workspace lifecycle convergence complete"
