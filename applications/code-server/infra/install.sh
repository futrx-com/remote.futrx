#!/usr/bin/env bash
set -euo pipefail
# Code Server is an optional project application. Remote owns its systemd service.
# Remote resolves this required input from the application manifest.
: "${CODE_SERVER_VERSION:?Code Server version is required}"
if [[ ! "$CODE_SERVER_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Code Server version must use major.minor.patch (for example 4.121.0)" >&2
    exit 1
fi
ARCH="$(dpkg --print-architecture)"
case "$ARCH" in amd64|arm64) ;; *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; esac
# An older project image may still have the legacy socket enabled.
systemctl disable --now code-server.socket 2>/dev/null || true
systemctl disable --now code-server-proxy.service 2>/dev/null || true
systemctl stop code-server.service 2>/dev/null || true
rm -f /etc/systemd/system/code-server.socket /etc/systemd/system/code-server-proxy.service

if ! command -v code-server >/dev/null 2>&1 \
   || [ "$(code-server --version 2>/dev/null | head -1 | awk '{print $1}')" != "$CODE_SERVER_VERSION" ]; then
    deb="$(mktemp --suffix=.deb)"
    trap 'rm -f "$deb"' EXIT
    curl -fsSL --retry 3 -o "$deb" \
        "https://github.com/coder/code-server/releases/download/v${CODE_SERVER_VERSION}/code-server_${CODE_SERVER_VERSION}_${ARCH}.deb"
    apt-get -o DPkg::Lock::Timeout=300 install -y -qq "$deb"
fi

# Remote stages infra/payload.tar.gz in APP_PACKAGE_DIR before running this script.
install -m 0644 "${APP_PACKAGE_DIR:?Missing application payload}/infra/remote-open.html" \
    /usr/lib/code-server/remote-open.html

install -d -m 0700 /root/.config/code-server
cat > /root/.config/code-server/config.yaml <<YAML
bind-addr: 0.0.0.0:8842
auth: none
cert: false
app-name: Futrx IDE - $(hostname)
YAML
chmod 0600 /root/.config/code-server/config.yaml

# Managed user settings for this container's code-server. Runtime keys an
# extension may add later (e.g. dbcode.connections) are workspace-specific and
# intentionally omitted here.
#
# Note: editor.experimentalGpuAcceleration stays "off" (upstream default). Its
# WebGPU renderer observes the editor canvas with
# ResizeObserver.observe(el, { box: ['device-pixel-content-box'] }), which
# WebKit does not implement -- observe() throws, VS Code rethrows it as
# "Could not observe device pixel dimensions", and the editor never mounts. On
# iOS/iPadOS every browser is WebKit, so turning this on breaks opening ANY
# file from a phone ("Unable to open '<file>'") while the explorer still
# renders. Settings here are server-side and shared by every client of this
# container, so there is no per-client opt-out -- desktop Chrome would have to
# cost mobile the editor entirely. Keep it off.
# Link the whole User directory: VS Code saves files with atomic rename, which
# would replace a settings.json symlink. All user settings now live durably.
export CODE_SERVER_WS_NAME="${CODE_SERVER_WS_NAME:-$(hostname)}"
node "$APP_PACKAGE_DIR/infra/migrate-settings.cjs"

# Pinned extensions, best-effort: a flaky Open VSX must never fail the build.
for ext in \
    anan.jetbrains-darcula-theme \
    anwar.papyrus-pdf \
    chuckjonas.duckdb \
    dbcode.dbcode \
    golang.go \
    onlyutkarsh.mermaid-diagram-lens \
    pkief.material-icon-theme \
    repreng.csv \
    ; do
    code-server --install-extension "$ext" >/dev/null 2>&1 || true
done
