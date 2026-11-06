#!/usr/bin/env bash
# Idempotent File Manager provisioner. Runs as root inside the project container.
#
# Versions before 5 served the drawer from a container web service. Remote stops
# managing a unit once the manifest no longer declares it, so upgraded
# containers retire it here.
set -euo pipefail

legacy=remote-file-manager
if [ -e "/etc/systemd/system/$legacy.service" ]; then
  systemctl disable --now "$legacy" || true
  rm -f "/etc/systemd/system/$legacy.service" "/etc/remote/workspace-idle.d/$legacy"
  rm -rf "/etc/remote/applications/$legacy"
  systemctl daemon-reload
fi
