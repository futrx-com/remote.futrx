#!/usr/bin/env bash
# Bake the futrx-remote-dev-base LXD image used by every project container.
# Driven by backend/cmd/build-base-image, which is the single source of
# truth for the install recipe (Node 22 + agent CLIs baked in).
#
# Idempotent: skips when the alias is already published. Set
# FORCE_REBUILD_BASE_IMAGE=1 to delete + rebuild even when present.
#
# Expects from caller:
#   - log / ok / warn / err helpers
#   - $INSTALL_DIR  (where the repo lives)
set -euo pipefail

step_05_base_image() {
BASE_IMAGE_ALIAS="${FUTRX_DEFAULT_BASE_IMAGE:-futrx-remote-dev-base}"

log "Base image: $BASE_IMAGE_ALIAS"

if ! command -v lxc >/dev/null; then
    err "lxc CLI not found — step 01 should have installed LXD."
    exit 1
fi

# Make sure the local LXD daemon is responsive before we try to publish.
# On a brand-new install it can take a few seconds for the snap socket to
# be ready after `lxd init`.
for _ in 1 2 3 4 5; do
    if lxc info --resources >/dev/null 2>&1; then
        break
    fi
    sleep 2
done

if [ -z "${FORCE_REBUILD_BASE_IMAGE:-}" ] \
   && lxc image list --format csv "$BASE_IMAGE_ALIAS" 2>/dev/null | grep -q "^${BASE_IMAGE_ALIAS},"; then
    ok "$BASE_IMAGE_ALIAS already published — skipping build"
    return 0 2>/dev/null || exit 0
fi

CLI_ARGS=()
if [ -n "${FORCE_REBUILD_BASE_IMAGE:-}" ]; then
    warn "FORCE_REBUILD_BASE_IMAGE set — rebuilding from scratch"
    CLI_ARGS+=(-overwrite)
fi

log "Running cmd/build-base-image (the first build can take up to 10 minutes)"
(
    cd "$INSTALL_DIR/backend"
    go run ./cmd/build-base-image "${CLI_ARGS[@]}"
)
ok "$BASE_IMAGE_ALIAS published"
}
