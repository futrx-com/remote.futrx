#!/usr/bin/env bash
set -euo pipefail
command -v curl >/dev/null && command -v jq >/dev/null || {
  apt-get update
  apt-get install -y --no-install-recommends curl jq
}
install -d -m 755 /workspace/scripts
install -m 755 "${APP_PACKAGE_DIR:?}/infra/remote-schedule" /workspace/scripts/remote-schedule
rm -f /workspace/scripts/.remote-schedule.sha256
