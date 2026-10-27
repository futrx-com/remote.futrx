#!/usr/bin/env bash
# Select the immutable application checkout before any manifest or catalog is
# consumed. Re-executing the selected installer keeps every later step on the
# same code, including direct install.sh repair runs.
#
# Expects from caller:
#   - log / err helpers
#   - remote_exec_selected_installer helper
#   - $INSTALL_DIR, $REPO_URL, $GITHUB_TOKEN
#   - $FUTRX_CHECKOUT_REF (optional; defaults to the latest release tag)
set -euo pipefail

step_00_checkout() {
# Current entry points always carry the selected ref alongside this guard.
# Older installers set only the guard after resetting to origin/main; treating
# that legacy handoff as complete could build an untagged main commit.
if [ "${FUTRX_INSTALL_CHECKOUT_SELECTED:-0}" = "1" ] && \
   [ -n "${FUTRX_CHECKOUT_REF:-}" ]; then
    return 0
fi

if [ -d "$INSTALL_DIR" ] && [ ! -d "$INSTALL_DIR/.git" ]; then
    err "$INSTALL_DIR exists but is not a git checkout. Remove it and re-run."
    exit 1
fi

CLONE_URL="$REPO_URL"
if [ -n "${GITHUB_TOKEN:-}" ]; then
    CLONE_URL="https://x-access-token:${GITHUB_TOKEN}@github.com/futrx-com/remote.futrx.git"
fi

# shellcheck source=../lib/release-version.sh
. "$INFRA_DIR/lib/release-version.sh"

REQUESTED_REF="${FUTRX_CHECKOUT_REF:-}"
if [ -d "$INSTALL_DIR/.git" ]; then
    git -C "$INSTALL_DIR" fetch --quiet --tags --prune --prune-tags origin
else
    log "Cloning repo to $INSTALL_DIR"
    if ! git clone --depth=1 --branch main --single-branch \
        "$CLONE_URL" "$INSTALL_DIR" 2>&1; then
        err "git clone failed."
        if [ -z "${GITHUB_TOKEN:-}" ]; then
            cat <<EOF >&2

  This repo is private. Provide a GitHub Personal Access Token:
    1. https://github.com/settings/personal-access-tokens → Generate (fine-grained)
    2. Select repo futrx-com/remote.futrx.com → Contents: Read
    3. Re-run with --github-token=ghp_xxx
EOF
        fi
        exit 1
    fi
    chmod 0600 "$INSTALL_DIR/.git/config"
    git -C "$INSTALL_DIR" fetch --quiet --tags --prune --prune-tags origin
fi

if [ -n "$REQUESTED_REF" ]; then
    CHECKOUT_REF="$REQUESTED_REF"
else
    if ! CHECKOUT_REF="$(release_latest_tag "$INSTALL_DIR")"; then
        err "No complete numeric release tag is available for installation."
        exit 1
    fi
fi

log "Selecting checkout at $INSTALL_DIR (ref: $CHECKOUT_REF)"
if ! git -C "$INSTALL_DIR" rev-parse --verify --quiet \
        "refs/tags/${CHECKOUT_REF}^{commit}" >/dev/null && \
   ! git -C "$INSTALL_DIR" rev-parse --verify --quiet \
        "${CHECKOUT_REF}^{commit}" >/dev/null; then
    git -C "$INSTALL_DIR" fetch --quiet --depth=1 origin "$CHECKOUT_REF"
fi
CHECKOUT_COMMIT="$(
    git -C "$INSTALL_DIR" rev-parse --verify --quiet \
        "refs/tags/${CHECKOUT_REF}^{commit}" \
    || git -C "$INSTALL_DIR" rev-parse --verify "${CHECKOUT_REF}^{commit}"
)"
git -C "$INSTALL_DIR" reset --hard "$CHECKOUT_COMMIT"

export FUTRX_CHECKOUT_REF="$CHECKOUT_REF"
export FUTRX_INSTALL_CHECKOUT_SELECTED=1
remote_exec_selected_installer "$INSTALL_DIR/infra/install.sh" "$@"
}
