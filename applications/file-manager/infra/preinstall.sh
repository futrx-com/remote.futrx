#!/usr/bin/env bash
# Retires what earlier File Manager versions left in the project container.
# Remote runs this before it builds the new container program, on every
# install, so each step is idempotent.
set -euo pipefail

# Before version 5 a container web service served the drawer, running
# /usr/local/bin/file-manager. The build replaces that binary, so the service
# stops first: left enabled, it would restart the new program with --port,
# which the new program rejects.
legacy=remote-file-manager
if [ -e "/etc/systemd/system/$legacy.service" ]; then
  systemctl disable --now "$legacy" || true
  rm -f "/etc/systemd/system/$legacy.service" "/etc/remote/workspace-idle.d/$legacy"
  rm -rf "/etc/remote/applications/$legacy"
  systemctl daemon-reload
fi

# Builds that returned ZIPs through stdout spooled them here first.
rm -f /tmp/remote-workspace-archive-*.zip
