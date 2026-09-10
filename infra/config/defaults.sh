#!/usr/bin/env bash
# Central shared defaults for the infra entry points
# (install.sh, update.sh, deploy-app.sh, upgrade-workspaces.sh).
#
# Source this file — it defines readonly constants only and executes
# nothing. Environment overrides keep working: every entry point applies
# its own ${VAR:-$FUTRX_DEFAULT_*} wiring, so an exported FUTRX_INSTALL_DIR
# (QA/tests) still wins over the default below.
#
# The curl|bash bootstrap block at the top of install.sh intentionally
# inlines its own copy of the install dir / repo URL instead of sourcing
# this file: no checkout exists on disk yet at that point.

# Re-source guard: readonly assignments fail on a second source.
if [ -n "${__FUTRX_DEFAULTS_SOURCED:-}" ]; then
    return 0 2>/dev/null || exit 0
fi
__FUTRX_DEFAULTS_SOURCED=1

readonly FUTRX_DEFAULT_INSTALL_DIR="/opt/remote.futrx"
readonly FUTRX_DEFAULT_LEGACY_INSTALL_DIR="/opt/remote.futrx.dev"
readonly FUTRX_DEFAULT_SERVICE_NAME="remote.futrx.service"
readonly FUTRX_DEFAULT_SERVICE_PORT="7682"
readonly FUTRX_DEFAULT_BASE_IMAGE="futrx-remote-dev-base"
readonly FUTRX_REPOSITORY_URL="https://github.com/futrx-com/remote.futrx.git"
