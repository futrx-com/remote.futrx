#!/usr/bin/env bash
# Host-level system dependencies: apt base, Node 22, Go, Caddy, agent CLIs, LXD.
# Idempotent — re-runs are fast no-ops when everything's already installed.
#
# Expects from caller:
#   - log / ok / warn / err helpers
#   - $INFRA_DIR (path to infra/ in the cloned repo)
#   - $HOSTNAME (for diagnostic messages only)
#   - $SKIP_DNS_CHECK (0 / 1)
#
# Sets in environment for later steps:
#   - $LXD_BRIDGE_IP (for the resolved drop-in)
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive

# ───────────────── base apt deps ─────────────────
log "apt update + base packages"
apt-get update -qq
apt-get install -y -qq git curl ca-certificates gnupg jq tmux gettext-base

# ───────────────── version pins ─────────────────
# infra/versions.env (a symlink to the canonical manifest embedded by the
# backend) declares the exact non-agent versions the host must run. Agent CLI
# releases are resolved, installed, and written back to this manifest later in
# this step so the backend and container image use exactly what the host got.
VERSIONS_FILE="$INFRA_DIR/versions.env"
if [ ! -r "$VERSIONS_FILE" ]; then
    err "missing version manifest: $VERSIONS_FILE"
    exit 1
fi
# shellcheck source=/dev/null
. "$VERSIONS_FILE"
for v in NODE_MAJOR NODE_MIN_VERSION GO_VERSION; do
    if [ -z "${!v:-}" ]; then
        err "version manifest is missing $v: $VERSIONS_FILE"
        exit 1
    fi
done

# ───────────────── Node (pinned major) ─────────────────
CURRENT_NODE_MAJOR=""
CURRENT_NODE_VERSION=""
if command -v node >/dev/null; then
    CURRENT_NODE_VERSION="$(node -v | sed 's/^v//')"
    CURRENT_NODE_MAJOR="${CURRENT_NODE_VERSION%%.*}"
fi
if [ "$CURRENT_NODE_MAJOR" != "$NODE_MAJOR" ] \
   || dpkg --compare-versions "${CURRENT_NODE_VERSION:-0}" lt "$NODE_MIN_VERSION"; then
    log "Installing Node ${NODE_MAJOR}.x (>=${NODE_MIN_VERSION}) from NodeSource (was ${CURRENT_NODE_VERSION:-missing})"
    curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash - >/dev/null
    apt-get install -y -qq nodejs
fi
ok "node $(node -v)  npm $(npm -v)"

# ───────────────── Go (pinned, official tarball) ─────────────────
# Installed to /usr/local/go with symlinks in /usr/local/bin, which
# precedes /usr/bin on PATH — so this safely upgrades older distro installs.
GO_TOOLCHAIN_INSTALLER="$INFRA_DIR/lib/go-toolchain.sh"
if [ ! -r "$GO_TOOLCHAIN_INSTALLER" ]; then
    err "missing Go toolchain installer: $GO_TOOLCHAIN_INSTALLER"
    exit 1
fi
# shellcheck source=/dev/null
. "$GO_TOOLCHAIN_INSTALLER"
ensure_go_toolchain "$GO_VERSION"

# ───────────────── ports 80/443 free? ─────────────────
log "Checking ports 80 + 443 are free (or held by Caddy)"
for p in 80 443; do
    if ss -tln 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${p}\$"; then
        owner=$(ss -tlnp 2>/dev/null | grep -E "[:.]${p} " | head -1 || true)
        if ! echo "$owner" | grep -q "caddy"; then
            err "Port ${p} is already in use by another process."
            cat <<EOF >&2

  $owner

  Caddy needs ports 80 and 443 exclusively. Stop the other service first:
    sudo ss -tlnp | grep ':$p '
    sudo systemctl stop <service>
  Then re-run the installer.
EOF
            exit 1
        fi
    fi
done
ok "ports 80 + 443 OK"

# ───────────────── Caddy ─────────────────
if ! command -v caddy >/dev/null; then
    log "Installing Caddy (Cloudsmith repo)"
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
        | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
    curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
        > /etc/apt/sources.list.d/caddy-stable.list
    apt-get update -qq
    apt-get install -y -qq caddy
fi
ok "$(caddy version | head -1)"

# ───────────────── agent CLIs (host-side auth/provisioning) ─────────────────
# Resolve the latest upstream releases on every install/update. Once installed,
# their reported versions are written to versions.env. The exact snapshot is
# then embedded by the backend and used to build/repair project containers.
AGENT_CLI_VERSION_LIB="$INFRA_DIR/lib/agent-cli-versions.sh"
if [ ! -r "$AGENT_CLI_VERSION_LIB" ]; then
    err "missing agent CLI version helper: $AGENT_CLI_VERSION_LIB"
    exit 1
fi
# shellcheck source=../lib/agent-cli-versions.sh
. "$AGENT_CLI_VERSION_LIB"

ensure_latest_npm_agent_cli() {
    local label="$1" binary="$2" package="$3" output_variable="$4"
    local expected current="" installed
    expected="$(latest_npm_package_version "$package")" || return 1
    if command -v "$binary" >/dev/null; then
        current="$(agent_cli_semver "$binary")"
    fi
    if [ "$current" != "$expected" ]; then
        log "Installing $label $expected (was ${current:-missing})"
        npm install -g "${package}@${expected}" --silent 2>&1 | tail -3
    fi
    installed="$(agent_cli_semver "$binary")"
    require_agent_cli_semver "$label" "$installed" || return 1
    if [ "$installed" != "$expected" ]; then
        err "$label reported $installed after installing $expected"
        return 1
    fi
    printf -v "$output_variable" '%s' "$installed"
    export "$output_variable"
    ok "$label $("$binary" --version 2>&1 | head -1)"
}

ensure_latest_npm_agent_cli \
    "Claude Code" claude @anthropic-ai/claude-code CLAUDE_CODE_VERSION
ensure_latest_npm_agent_cli \
    "Codex" codex @openai/codex CODEX_CLI_VERSION
ensure_latest_npm_agent_cli \
    "Kimi Code" kimi @moonshot-ai/kimi-code KIMI_CODE_VERSION

ANTIGRAVITY_CLI_VERSION="$(latest_antigravity_cli_version)"
export ANTIGRAVITY_CLI_VERSION
ok "Antigravity CLI $ANTIGRAVITY_CLI_VERSION available for container installation"

write_agent_cli_versions "$VERSIONS_FILE" \
    "$CLAUDE_CODE_VERSION" \
    "$CODEX_CLI_VERSION" \
    "$KIMI_CODE_VERSION" \
    "$ANTIGRAVITY_CLI_VERSION"
ok "agent CLI versions recorded in $VERSIONS_FILE"

# ───────────────── LXD (one container per project) ─────────────────
if ! command -v lxc >/dev/null; then
    log "Installing LXD via snap"
    if ! command -v snap >/dev/null; then
        apt-get install -y -qq snapd
        systemctl enable --now snapd.socket
        for _ in 1 2 3 4 5; do snap wait system seed.loaded && break; sleep 1; done
    fi
    snap install lxd
    export PATH="/snap/bin:$PATH"
fi

# Initialize storage + bridge on fresh installs. `lxc network show lxdbr0`
# is our "initialized" probe.
if ! lxc network show lxdbr0 >/dev/null 2>&1; then
    log "lxd init --auto"
    lxd init --auto
fi
ok "lxd $(lxc version --format=csv 2>/dev/null | tr ',' ' ' | awk '{print $1}' || echo ok)"

LXD_BRIDGE="lxdbr0"
export LXD_BRIDGE

# ───────────────── container IPv4 egress ─────────────────
# Docker drops forwarded IPv4 by default, which strands containers on IPv6-only
# reachability without any obvious symptom. See lib/container-forwarding.sh.
# shellcheck source=../lib/container-forwarding.sh
. "$INFRA_DIR/lib/container-forwarding.sh"
if container_forwarding_needed; then
    FORWARD_CHAIN="$(container_forwarding_chain)"
    log "FORWARD chain is restricted — allowing $LXD_BRIDGE through $FORWARD_CHAIN"
    if FORWARD_RULES_ADDED="$(ensure_container_forwarding "$LXD_BRIDGE")"; then
        if [ -n "$FORWARD_RULES_ADDED" ]; then
            ok "containers can reach IPv4 (added $FORWARD_CHAIN rules)"
        else
            ok "containers can reach IPv4 ($FORWARD_CHAIN already allows $LXD_BRIDGE)"
        fi
    else
        err "Could not allow $LXD_BRIDGE through $FORWARD_CHAIN."
        echo "  Containers would have no IPv4 egress, and the base image build would" >&2
        echo "  fail minutes later on the first IPv4-only host it needs." >&2
        exit 1
    fi
fi

# Re-apply on every boot: the rules are not persistent and Docker reinstates
# its policy on each start. Installed unconditionally; the unit self-gates.
log "Rendering /etc/systemd/system/futrx-lxd-forward.service"
render_template "${INFRA_DIR}/templates/futrx-lxd-forward.service.tmpl" \
                /etc/systemd/system/futrx-lxd-forward.service
systemctl daemon-reload
systemctl enable --now futrx-lxd-forward.service >/dev/null 2>&1 \
    || warn "futrx-lxd-forward.service could not be enabled; container IPv4 egress may not survive a reboot."

# Detect the bridge IP so the resolved drop-in can forward *.lxd queries.
LXD_BRIDGE_IP=$(lxc network get "$LXD_BRIDGE" ipv4.address 2>/dev/null | sed 's|/.*||')
if [ -z "$LXD_BRIDGE_IP" ]; then
    warn "lxdbr0 bridge IP not detectable — *.dev.${HOSTNAME} routing will fail."
else
    export LXD_BRIDGE_IP
fi

# ───────────────── systemd-resolved: forward *.lxd to the bridge ─────────────────
if [ -n "${LXD_BRIDGE_IP:-}" ] && systemctl is-active --quiet systemd-resolved; then
    log "systemd-resolved drop-in for *.lxd → ${LXD_BRIDGE_IP}"
    mkdir -p /etc/systemd/resolved.conf.d
    render_template "${INFRA_DIR}/templates/lxd-resolved.conf.tmpl" \
                    /etc/systemd/resolved.conf.d/lxd.conf
    systemctl restart systemd-resolved
fi
