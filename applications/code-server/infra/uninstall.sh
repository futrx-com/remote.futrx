#!/usr/bin/env bash
set -euo pipefail

# Remote has already stopped and disabled the service. Purge the
# package installed by this application, including a partial install left by
# an interrupted older request. A missing package is safe to uninstall again.
if dpkg-query -W code-server >/dev/null 2>&1; then
    apt-get -o DPkg::Lock::Timeout=300 purge -y -qq code-server
fi

rm -f -- /usr/lib/code-server/remote-open.html

rm -rf -- \
    /root/.config/code-server \
    /root/.local/share/code-server \
    /root/.local/state/code-server \
    /root/.cache/code-server \
    /workspace/.remote/code-server
