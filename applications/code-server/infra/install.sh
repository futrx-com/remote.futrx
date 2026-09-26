#!/usr/bin/env bash
set -euo pipefail
# Code Server is an optional project application. Remote owns its systemd units.
CODE_SERVER_VERSION=4.121.0
ARCH="$(dpkg --print-architecture)"
case "$ARCH" in amd64|arm64) ;; *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;; esac
if ! command -v code-server >/dev/null 2>&1 \
   || [ "$(code-server --version 2>/dev/null | head -1 | awk '{print $1}')" != "$CODE_SERVER_VERSION" ]; then
    deb="$(mktemp --suffix=.deb)"
    trap 'rm -f "$deb"' EXIT
    curl -fsSL --retry 3 -o "$deb" \
        "https://github.com/coder/code-server/releases/download/v${CODE_SERVER_VERSION}/code-server_${CODE_SERVER_VERSION}_${ARCH}.deb"
    apt-get -o DPkg::Lock::Timeout=300 install -y -qq "$deb"
fi

# An older project image may still have the legacy socket enabled.
systemctl disable --now code-server.socket 2>/dev/null || true
systemctl stop code-server-proxy.service code-server.service 2>/dev/null || true

install -d -m 0700 /root/.config/code-server
cat > /root/.config/code-server/config.yaml <<'YAML'
bind-addr: 127.0.0.1:8081
auth: none
cert: false
app-name: Futrx IDE
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
install -d -m 0755 /root/.local/share/code-server/User
# The workspace is a durable mount, so this application-owned copy survives
# container replacement. The host backend maintains it after installation.
# The backend replaces a leftover copy from an uninstalled app with the new
# install form after this script finishes.
export CODE_SERVER_WS_NAME="${CODE_SERVER_WS_NAME:-$(hostname)}"
node <<'NODE'
const fs = require("fs");
const durable = "/workspace/.remote/code-server/settings.json";
const active = "/root/.local/share/code-server/User/settings.json";
const source = fs.existsSync(durable)
  ? fs.readFileSync(durable, "utf8")
  : process.env.CODE_SERVER_SETTINGS_JSON;
const settings = JSON.parse(source || "null");
if (!settings || typeof settings !== "object" || Array.isArray(settings)) {
  throw new Error("Code Server settings must be a JSON object");
}
if (settings["window.title"] === "${rootPath}") {
  settings["window.title"] = process.env.CODE_SERVER_WS_NAME;
}
const content = JSON.stringify(settings, null, 2) + "\n";
for (const path of [durable, active]) {
  fs.mkdirSync(require("path").dirname(path), { recursive: true, mode: 0o700 });
  const temporary = path + ".remote-tmp";
  fs.writeFileSync(temporary, content, { mode: 0o600 });
  fs.renameSync(temporary, path);
}
NODE

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
