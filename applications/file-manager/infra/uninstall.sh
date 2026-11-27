#!/usr/bin/env bash
set -euo pipefail
# The workspace belongs to the project. A copy installed before version 5 may
# still carry the retired container web service, so remove that too.
legacy=remote-file-manager
if [ -e "/etc/systemd/system/$legacy.service" ]; then
  systemctl disable --now "$legacy" || true
  rm -f "/etc/systemd/system/$legacy.service" "/etc/remote/workspace-idle.d/$legacy"
  rm -rf "/etc/remote/applications/$legacy"
  systemctl daemon-reload
fi
rm -f /usr/local/bin/file-manager /usr/local/lib/remote/file-manager.build
# Folder ZIPs waiting for the browser are File Manager's, not the project's.
rm -rf /workspace/.file-manager-downloads
