#!/usr/bin/env bash
# remote.futrx — production application-only release deployer.
#
# Use this to deploy frontend/backend code when the installed and target
# releases have the same MAJOR.MINOR version (for example, 0.4.1 -> 0.4.2).
# The in-app updater selects this script automatically for that case; the
# command below is the manual operator equivalent.
#
# The target must resolve as a release tag after the script fetches tags from
# the installed checkout's origin. It builds the frontend and a staged backend
# binary, replaces the live binary, restarts remote.futrx.service, and verifies
# the local HTTP endpoint. If the build, restart, or health check fails, it
# restores the previous checkout and, if replaced, the previous binary.
#
# This script does NOT converge host dependencies, Caddy, systemd units, LXD,
# the workspace base image, or project containers. It rejects major/minor
# crossings, unknown installed versions, and legacy two-component versions;
# use infra/update.sh for those infrastructure updates.
#
# Requirements: an existing installed service, root privileges, and the Git,
# Node/npm, and Go toolchains previously installed by infra/install.sh.
#
# Usage:
#   sudo bash /opt/remote.futrx/infra/deploy-app.sh --ref=<release-tag>
#
# Test-only overrides: FUTRX_INSTALL_DIR, FUTRX_SERVICE_NAME, FUTRX_SERVICE_PORT.
set -euo pipefail

usage() {
    sed -n '2,/^set -euo pipefail$/ { /^set -euo pipefail$/d; s/^# \{0,1\}//p; }' "$0"
}

TARGET_REF=""
for argument in "$@"; do
    case "$argument" in
        --ref=*)  TARGET_REF="${argument#*=}" ;;
        -h|--help) usage; exit 0 ;;
        --*)      echo "unknown flag: $argument" >&2; exit 2 ;;
        *)        echo "unexpected argument: $argument" >&2; exit 2 ;;
    esac
done

if [ -z "$TARGET_REF" ]; then
    echo "--ref=<release-tag> is required" >&2
    exit 2
fi
if [ "$EUID" -ne 0 ]; then
    echo "this application deployer needs root; rerun with sudo" >&2
    exit 1
fi

DEFAULT_INSTALL_DIR="${INSTALL_DIR:-/opt/remote.futrx}"
INSTALL_DIR="${FUTRX_INSTALL_DIR:-$DEFAULT_INSTALL_DIR}"
SERVICE_NAME="${FUTRX_SERVICE_NAME:-remote.futrx.service}"
DEFAULT_SERVICE_PORT="${PORT:-7682}"
SERVICE_PORT="${FUTRX_SERVICE_PORT:-$DEFAULT_SERVICE_PORT}"
BINARY="$INSTALL_DIR/backend/remote"

if [ ! -d "$INSTALL_DIR/.git" ]; then
    echo "$INSTALL_DIR is not an installed git checkout; run infra/install.sh first" >&2
    exit 1
fi
if ! systemctl cat "$SERVICE_NAME" >/dev/null 2>&1; then
    echo "$SERVICE_NAME is not installed; run infra/install.sh first" >&2
    exit 1
fi
if [ ! -x "$BINARY" ]; then
    echo "installed application binary is missing or not executable: $BINARY" >&2
    exit 1
fi
for command_name in git npm go systemctl; do
    command -v "$command_name" >/dev/null 2>&1 || {
        echo "missing required command: $command_name" >&2
        echo "run infra/update.sh to converge host dependencies" >&2
        exit 1
    }
done

SCRIPT_INFRA_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
# shellcheck source=lib/release-version.sh
. "$SCRIPT_INFRA_DIR/lib/release-version.sh"

cd "$INSTALL_DIR"
PREVIOUS_SHA="$(git rev-parse --verify 'HEAD^{commit}')"
git fetch --quiet --tags origin
CURRENT_VERSION="$(git describe --tags --abbrev=0 --match '[0-9]*' --match 'v[0-9]*' HEAD 2>/dev/null || true)"
TARGET_COMMIT="$(git rev-parse --verify --quiet "refs/tags/${TARGET_REF}^{commit}" || true)"
if [ -z "$TARGET_COMMIT" ]; then
    echo "release tag is unavailable after fetching origin: $TARGET_REF" >&2
    exit 1
fi

UPDATE_KIND="$(release_update_kind "$CURRENT_VERSION" "$TARGET_REF")"
if [ "$UPDATE_KIND" != "application" ]; then
    echo "refusing application-only deployment from ${CURRENT_VERSION:-unknown} to $TARGET_REF" >&2
    echo "major/minor changes require: sudo bash $INSTALL_DIR/infra/update.sh --ref=$TARGET_REF" >&2
    exit 1
fi

STAGE_DIR="$(mktemp -d "$INSTALL_DIR/backend/.app-deploy.XXXXXX")"
PREVIOUS_BINARY="$STAGE_DIR/previous"
STAGED_BINARY="$STAGE_DIR/remote"
cp -p "$BINARY" "$PREVIOUS_BINARY"
BINARY_REPLACED=0
DEPLOYMENT_SUCCEEDED=0

finish() {
    local status="$1"
    trap - EXIT
    if [ "$DEPLOYMENT_SUCCEEDED" -ne 1 ]; then
        echo "Application deployment failed; restoring the previous release" >&2
        git reset --hard "$PREVIOUS_SHA" >/dev/null 2>&1 || true
        if [ "$BINARY_REPLACED" -eq 1 ]; then
            install -m 0755 "$PREVIOUS_BINARY" "$BINARY" || true
            systemctl restart "$SERVICE_NAME" || true
        fi
    fi
    rm -f "$PREVIOUS_BINARY" "$STAGED_BINARY"
    rmdir "$STAGE_DIR" 2>/dev/null || true
    exit "$status"
}
trap 'finish $?' EXIT

echo "==> Deploying application release $TARGET_REF ($TARGET_COMMIT)"
echo "    infrastructure, base image, and project containers will be unchanged"
git reset --hard "$TARGET_COMMIT"

echo "==> Building frontend"
(
    cd frontend
    npm install --silent --no-audit --no-fund 2>&1 | tail -3
    npm run build
)

echo "==> Building backend"
APP_VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
(
    cd backend
    go build -trimpath \
        -ldflags="-s -w -X github.com/futrx-com/remote.futrx.com/internal/version.Version=${APP_VERSION}" \
        -o "$STAGED_BINARY" ./cmd/remote
)

install -m 0755 "$STAGED_BINARY" "$BINARY"
BINARY_REPLACED=1
systemctl restart "$SERVICE_NAME"

# shellcheck source=lib/health-check.sh
. "$INSTALL_DIR/infra/lib/health-check.sh"
wait_for_http_health "http://127.0.0.1:${SERVICE_PORT}/" 30
systemctl is-active --quiet "$SERVICE_NAME"

DEPLOYED_SHA="$(git rev-parse --verify 'HEAD^{commit}')"
if [ "$DEPLOYED_SHA" != "$TARGET_COMMIT" ]; then
    echo "deployed $DEPLOYED_SHA, expected $TARGET_COMMIT" >&2
    exit 1
fi

DEPLOYMENT_SUCCEEDED=1
echo
echo "✓ application release $TARGET_REF deployed"
