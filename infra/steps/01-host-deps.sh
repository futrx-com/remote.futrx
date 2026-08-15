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
# backend) declares the exact versions the host must run. Every section
# below converges the live box to its pin instead of only checking
# existence — bump a pin and re-run to upgrade.
VERSIONS_FILE="$INFRA_DIR/versions.env"
if [ ! -r "$VERSIONS_FILE" ]; then
    err "missing version manifest: $VERSIONS_FILE"
    exit 1
fi
# shellcheck source=/dev/null
. "$VERSIONS_FILE"
for v in NODE_MAJOR NODE_MIN_VERSION GO_VERSION \
         CLAUDE_CODE_VERSION CODEX_CLI_VERSION KIMI_CODE_VERSION; do
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

# ───────────────── ports ─────────────────
port_owner() {
    ss -tlnp 2>/dev/null | grep -E "[:.]${1} " | head -1 || true
}
port_in_use() {
    ss -tln 2>/dev/null | awk '{print $4}' | grep -qE "[:.]${1}\$"
}

if [ "${FUTRX_FRONTEND_MODE:-standalone}" = "plesk" ]; then
    # Plesk owns 80 and 443 and is not going to give them up, so Caddy moves
    # behind its nginx on a loopback port instead. What has to be free is that
    # port. See steps/08-plesk-frontend.sh.
    log "Plesk detected — checking port ${CADDY_HTTP_PORT} is free for Caddy"
    if port_in_use "$CADDY_HTTP_PORT"; then
        owner="$(port_owner "$CADDY_HTTP_PORT")"
        if ! echo "$owner" | grep -q "caddy"; then
            err "Port ${CADDY_HTTP_PORT} is already in use by another process."
            cat <<EOF >&2

  $owner

  Caddy needs a free loopback port to sit behind Plesk's nginx. Either stop
  that process, or pick another port:
    sudo bash infra/install.sh ${HOSTNAME} --caddy-port=8081
EOF
            exit 1
        fi
    fi
    if ! port_in_use 443; then
        warn "Nothing is listening on 443 — is Plesk's nginx running?"
        warn "The platform will not be reachable until it is."
    fi
    ok "port ${CADDY_HTTP_PORT} OK (Plesk keeps 80 + 443)"
else
    log "Checking ports 80 + 443 are free (or held by Caddy)"
    for p in 80 443; do
        if port_in_use "$p"; then
            owner="$(port_owner "$p")"
            if ! echo "$owner" | grep -q "caddy"; then
                err "Port ${p} is already in use by another process."
                cat <<EOF >&2

  $owner

  Caddy needs ports 80 and 443 exclusively. Stop the other service first:
    sudo ss -tlnp | grep ':$p '
    sudo systemctl stop <service>
  Then re-run the installer.

  If this is a control panel that will not release them (Plesk, cPanel),
  the installer can run behind it instead — it detects Plesk automatically.
  If you previously passed --no-plesk-integration, that choice is remembered
  in /etc/remote.futrx/install-options.env; undo it with --plesk-integration.
EOF
                exit 1
            fi
        fi
    done
    ok "ports 80 + 443 OK"
fi

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
# Pins come from the same versions.env sourced above (also embedded by the Go
# container manager). Re-running the installer upgrades stale host binaries
# instead of only checking existence.
agent_cli_version() {
    "$1" --version 2>&1 \
        | grep -Eo '[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?' \
        | head -1 || true
}

ensure_agent_cli() {
    local label="$1" binary="$2" package="$3" expected="$4" current=""
    if command -v "$binary" >/dev/null; then
        current="$(agent_cli_version "$binary")"
    fi
    if [ "$current" != "$expected" ]; then
        log "Installing $label $expected (was ${current:-missing})"
        npm install -g "${package}@${expected}" --silent 2>&1 | tail -3
    fi
    ok "$label $("$binary" --version 2>&1 | head -1)"
}

ensure_agent_cli "Claude Code" claude @anthropic-ai/claude-code "$CLAUDE_CODE_VERSION"
ensure_agent_cli "Codex" codex @openai/codex "$CODEX_CLI_VERSION"
ensure_agent_cli "Kimi Code" kimi @moonshot-ai/kimi-code "$KIMI_CODE_VERSION"

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
# Containers that lose IPv4 keep IPv6, so they reach every AAAA destination and
# the build only dies minutes later on the first IPv4-only host it needs. At
# least four separate host conditions cause it, so nothing here tries to infer
# which one applies — it converges all of them. See lib/container-forwarding.sh.
# shellcheck source=../lib/container-forwarding.sh
. "$INFRA_DIR/lib/container-forwarding.sh"

log "Converging container IPv4 egress"

if FORWARD_CHANGED="$(ensure_ip_forwarding)"; then
    [ -z "$FORWARD_CHANGED" ] || ok "IPv4 forwarding enabled ($FORWARD_CHANGED)"
else
    err "Could not enable net.ipv4.ip_forward — containers will have no IPv4 egress."
    exit 1
fi

if BRIDGE_CHANGED="$(ensure_bridge_nat "$LXD_BRIDGE")"; then
    [ -z "$BRIDGE_CHANGED" ] || ok "bridge reconfigured ($BRIDGE_CHANGED)"
else
    warn "Could not verify $LXD_BRIDGE's IPv4 subnet and NAT; check 'lxc network show $LXD_BRIDGE'."
fi

# Applied unconditionally. The previous version only acted when the FORWARD
# policy was DROP or a DOCKER-USER chain existed, which meant a host blocked by
# a rule, by the other iptables backend, or by a sysctl was silently skipped —
# and the install then failed identically on every re-run. The rules are scoped
# to our own bridge and idempotent, so applying them where they were not needed
# costs nothing.
if FORWARD_RULES_ADDED="$(ensure_container_forwarding "$LXD_BRIDGE")"; then
    if [ -n "$FORWARD_RULES_ADDED" ]; then
        ok "containers can reach IPv4 (added $(printf '%s' "$FORWARD_RULES_ADDED" | grep -c .) rules)"
    else
        ok "containers can reach IPv4 (forwarding rules already in place)"
    fi
else
    err "Could not add the forwarding rules for $LXD_BRIDGE."
    echo "  Containers would have no IPv4 egress, and the base image build would" >&2
    echo "  fail minutes later on the first IPv4-only host it needs." >&2
    echo "  Run: sudo bash ${INFRA_DIR}/diagnose-network.sh" >&2
    exit 1
fi

# A native nftables drop is not something we can safely override: every table
# sees the packet and any drop wins, so an accept in a table of our own would
# not rescue it. Report it precisely rather than claim a fix that did not land.
NFT_DROPS="$(detect_nft_forward_drop || true)"
if [ -n "$NFT_DROPS" ]; then
    warn "A native nftables ruleset drops forwarded traffic; our iptables rules cannot override it."
    while read -r nft_family nft_table nft_chain; do
        [ -n "$nft_family" ] || continue
        echo "    table $nft_family $nft_table chain $nft_chain — allow the bridge with:" >&2
        echo "      nft insert rule $nft_family $nft_table $nft_chain iifname \"$LXD_BRIDGE\" accept" >&2
        echo "      nft insert rule $nft_family $nft_table $nft_chain oifname \"$LXD_BRIDGE\" accept" >&2
    done <<< "$NFT_DROPS"
fi

# Re-apply at boot and then every couple of minutes: the rules are not
# persistent, Docker reinstates its policy on each start, and a host firewall
# that reconciles its ruleset (Plesk's firewall module and fail2ban both do)
# removes them mid-life with no other symptom.
log "Rendering /etc/systemd/system/futrx-lxd-forward.{service,timer}"
render_template "${INFRA_DIR}/templates/futrx-lxd-forward.service.tmpl" \
                /etc/systemd/system/futrx-lxd-forward.service
render_template "${INFRA_DIR}/templates/futrx-lxd-forward.timer.tmpl" \
                /etc/systemd/system/futrx-lxd-forward.timer
systemctl daemon-reload
systemctl enable futrx-lxd-forward.service >/dev/null 2>&1 \
    || warn "futrx-lxd-forward.service could not be enabled; container IPv4 egress may not survive a reboot."
systemctl enable --now futrx-lxd-forward.timer >/dev/null 2>&1 \
    || warn "futrx-lxd-forward.timer could not be enabled; the rules will not be re-applied if a firewall removes them."

# Detect the bridge IP so the resolved drop-in can forward *.lxd queries.
LXD_BRIDGE_IP=$(lxc network get "$LXD_BRIDGE" ipv4.address 2>/dev/null | sed 's|/.*||')
if [ -z "$LXD_BRIDGE_IP" ]; then
    warn "lxdbr0 bridge IP not detectable — *.dev.${HOSTNAME} routing will fail."
else
    export LXD_BRIDGE_IP
fi

# ───────────────── DHCP + DNS on the bridge ─────────────────
# LXD serves container DHCP from a dnsmasq bound to the bridge address. If
# something else already holds :53 there it never starts, and containers boot
# with IPv6 link-local only — which looks exactly like the firewall failure
# above, is not, and is not helped by any of it. On a Plesk box the squatter is
# BIND: its default listen-on { any; } binds every interface address it finds,
# including the bridge. See lib/bridge-dns.sh.
if [ -n "${LXD_BRIDGE_IP:-}" ]; then
    # shellcheck source=../lib/bridge-dns.sh
    . "$INFRA_DIR/lib/bridge-dns.sh"
    log "Checking DHCP/DNS on ${LXD_BRIDGE_IP}:53"
    if ensure_bridge_dns "$LXD_BRIDGE" "$LXD_BRIDGE_IP"; then
        ok "LXD's dnsmasq serves ${LXD_BRIDGE_IP}:53"
    else
        err "Containers will not receive an IPv4 address."
        echo "  Nothing usable is serving DHCP on ${LXD_BRIDGE_IP}:53, so every container" >&2
        echo "  will come up with IPv6 only and the base image build will fail." >&2
        echo "  Run: sudo bash ${INFRA_DIR}/diagnose-network.sh" >&2
        exit 1
    fi
fi

# ───────────────── systemd-resolved: forward *.lxd to the bridge ─────────────────
if [ -n "${LXD_BRIDGE_IP:-}" ] && systemctl is-active --quiet systemd-resolved; then
    log "systemd-resolved drop-in for *.lxd → ${LXD_BRIDGE_IP}"
    mkdir -p /etc/systemd/resolved.conf.d
    render_template "${INFRA_DIR}/templates/lxd-resolved.conf.tmpl" \
                    /etc/systemd/resolved.conf.d/lxd.conf
    systemctl restart systemd-resolved
fi
