#!/usr/bin/env bash
# remote.futrx — one-command update for an installed box.
#
# The updater deliberately pulls and re-executes itself before doing any
# convergence. That guarantees newly committed toolchain pins and installer
# logic are used on this run. Agent CLI releases are resolved automatically by
# install.sh and embedded in the backend built during the same run.
#
# Default flow:
#   1. Reset /opt/remote.futrx to origin/main (or the --ref target).
#   2. Resolve and install current host agent CLIs, recording their versions.
#   3. Build and restart the application.
#   4. Rebuild the base image with those exact agent CLI versions.
#   5. Recycle idle workspaces onto the new image (busy ones are skipped).
#
# Usage:
#   sudo bash /opt/remote.futrx/infra/update.sh
#   sudo bash /opt/remote.futrx/infra/update.sh <hostname>
#   sudo bash /opt/remote.futrx/infra/update.sh --include-busy
#
# Flags:
#   --include-busy     also recycle workspaces with an active agent process
#   --ref=<ref>        update to a release tag (or any ref) instead of origin/main
#   --skip-workspaces  update the host/application without rebuilding the base
#                      image or recycling workspace containers
set -euo pipefail

INSTALL_DIR="${FUTRX_INSTALL_DIR:-/opt/remote.futrx}"
LEGACY_INSTALL_DIR="${FUTRX_LEGACY_INSTALL_DIR:-/opt/remote.futrx.dev}"
UNIT="${FUTRX_SERVICE_UNIT_PATH:-/etc/systemd/system/remote.futrx.service}"
LEGACY_UNIT="${FUTRX_LEGACY_SERVICE_UNIT_PATH:-/etc/systemd/system/remote.futrx.dev.service}"

usage() {
    sed -n '2,25s/^# \{0,1\}//p' "$0"
}

HOSTNAME=""
INCLUDE_BUSY=0
UPDATE_WORKSPACES=1
TARGET_REF=""
for a in "$@"; do
    case "$a" in
        --include-busy)    INCLUDE_BUSY=1 ;;
        --ref=*)           TARGET_REF="${a#*=}" ;;
        --skip-workspaces) UPDATE_WORKSPACES=0 ;;
        -h|--help)         usage; exit 0 ;;
        --*)               echo "unknown flag: $a" >&2; exit 1 ;;
        *)
            if [ -n "$HOSTNAME" ]; then
                echo "unexpected argument: $a" >&2
                exit 1
            fi
            HOSTNAME="$a"
            ;;
    esac
done

if [ "$EUID" -ne 0 ]; then
    echo "this updater needs root; rerun with sudo" >&2
    exit 1
fi

SCRIPT_INFRA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=lib/install-migration.sh
. "$SCRIPT_INFRA_DIR/lib/install-migration.sh"
migrate_legacy_install_dir "$INSTALL_DIR" "$LEGACY_INSTALL_DIR"

if [ ! -d "$INSTALL_DIR/.git" ]; then
    echo "$INSTALL_DIR is not an installed git checkout; run infra/install.sh first" >&2
    exit 1
fi

# update.sh can itself change in origin/main. Pull once, then hand control to
# the freshly checked-out copy before reading manifests or invoking install.sh.
if [ "${FUTRX_UPDATE_REEXECED:-0}" != "1" ]; then
    UPDATE_REF="${TARGET_REF:-origin/main}"
    echo "==> Updating repository at $INSTALL_DIR (ref: $UPDATE_REF)"
    git -C "$INSTALL_DIR" fetch --quiet --tags origin
    # Tags win over identically named branches: --ref pins a release.
    UPDATE_COMMIT="$(git -C "$INSTALL_DIR" rev-parse --verify --quiet "refs/tags/${UPDATE_REF}^{commit}" \
        || git -C "$INSTALL_DIR" rev-parse --verify "${UPDATE_REF}^{commit}")"
    git -C "$INSTALL_DIR" reset --hard "$UPDATE_COMMIT"
    export FUTRX_UPDATE_REEXECED=1
    exec bash "$INSTALL_DIR/infra/update.sh" "$@"
fi

INFRA_DIR="$INSTALL_DIR/infra"
if [ -z "$HOSTNAME" ]; then
    # The installer renders BASE_URL=https://<hostname> into the unit. During
    # a rename migration, only the legacy unit may exist yet.
    HOSTNAME="$(installed_hostname_from_units "$UNIT" "$LEGACY_UNIT" || true)"
fi
if [ -z "$HOSTNAME" ]; then
    echo "could not detect hostname from $UNIT or $LEGACY_UNIT — pass it explicitly:" >&2
    echo "  sudo bash $0 <hostname>" >&2
    exit 1
fi

# Keep install.sh's checkout step (steps/02-app.sh) on the same ref this
# updater just checked out, instead of resetting back to origin/main.
export FUTRX_CHECKOUT_REF="${TARGET_REF:-origin/main}"

if [ "$UPDATE_WORKSPACES" -eq 1 ]; then
    # Rebuild once in install.sh, after the new backend has been built. The
    # workspace upgrader then only needs to recycle containers onto that image.
    FORCE_REBUILD_BASE_IMAGE=1 \
        bash "$INFRA_DIR/install.sh" "$HOSTNAME" --skip-dns-check

    WORKSPACE_ARGS=(--no-rebake)
    if [ "$INCLUDE_BUSY" -eq 1 ]; then
        WORKSPACE_ARGS+=(--include-busy)
    fi
    bash "$INFRA_DIR/upgrade-workspaces.sh" "${WORKSPACE_ARGS[@]}"
else
    bash "$INFRA_DIR/install.sh" "$HOSTNAME" --skip-dns-check
fi

echo
echo "✓ update complete"
