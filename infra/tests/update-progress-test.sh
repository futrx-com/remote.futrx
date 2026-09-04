#!/usr/bin/env bash
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
UPGRADE_SCRIPT="$TESTS_DIR/../upgrade-workspaces.sh"
UPDATE_SCRIPT="$TESTS_DIR/../update.sh"
DEPLOY_SCRIPT="$TESTS_DIR/../deploy-app.sh"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf -- "$TEST_DIR"' EXIT

# shellcheck source=../lib/update-progress.sh
. "$TESTS_DIR/../lib/update-progress.sh"

export FUTRX_UPDATE_PROGRESS_PATH="$TEST_DIR/state/progress.json"
write_update_progress 'workspace-migration' 'Recycling "important" workspaces'

[ "$(stat -c '%a' "$FUTRX_UPDATE_PROGRESS_PATH" 2>/dev/null || stat -f '%Lp' "$FUTRX_UPDATE_PROGRESS_PATH")" = "600" ]
grep -Fq '"phase":"workspace-migration"' "$FUTRX_UPDATE_PROGRESS_PATH"
grep -Fq '"message":"Recycling \"important\" workspaces"' "$FUTRX_UPDATE_PROGRESS_PATH"
grep -Eq '"updatedAt":[0-9]+' "$FUTRX_UPDATE_PROGRESS_PATH"

grep -Fq 'maintenance.json' "$UPGRADE_SCRIPT"
grep -Fq -- '--progress-file' "$UPGRADE_SCRIPT"
if grep -Eq 'systemctl[[:space:]]+stop' "$UPGRADE_SCRIPT"; then
    echo "workspace upgrader still stops the control plane" >&2
    exit 1
fi
grep -Fq 'lib/update-progress.sh' "$UPDATE_SCRIPT"
grep -Fq 'lib/update-progress.sh' "$DEPLOY_SCRIPT"

echo "update progress writer tests passed"
