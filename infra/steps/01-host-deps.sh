#!/usr/bin/env bash
# Host-level system dependencies: apt base, Node 22, Go, Caddy, and LXD.
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

# ───────────────── swap (spike buffer) ─────────────────
# Running several dev servers at once produces a large, short-lived RSS spike
# (~5x steady state) that a swapless box can't survive — it OOM-kills the dev
# servers. Provision a swap file sized from RAM and free disk before the
# memory-heavy work below (Go build, base-image build). Skips gracefully when
# swap already exists or the disk has no room. See lib/swap-provision.sh.
# shellcheck source=../lib/swap-provision.sh
. "$INFRA_DIR/lib/swap-provision.sh"
ensure_swap

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

# ───────────────── LXD (one container per project) ─────────────────
LXD_HOST_HELPERS="$INFRA_DIR/lib/lxd-host.sh"
if [ ! -r "$LXD_HOST_HELPERS" ]; then
    err "missing LXD host helpers: $LXD_HOST_HELPERS"
    exit 1
fi
# shellcheck source=../lib/lxd-host.sh
. "$LXD_HOST_HELPERS"

NESTED_UNPRIVILEGED_LXC=0
LXD_IDMAP_BASE=1000000
LXD_IDMAP_SIZE=65536
if unprivileged_lxc_host; then
    NESTED_UNPRIVILEGED_LXC=1

    # Remote deliberately keeps every project container unprivileged. The
    # backend also chowns bind-mounted project data to this exact LXD idmap.
    # An outer Proxmox LXC must therefore delegate the complete range; falling
    # back to privileged inner containers would remove a primary isolation
    # boundary and would make the existing ownership contract incorrect.
    if ! nested_lxd_idmap_available \
        /proc/self/uid_map /proc/self/gid_map "$LXD_IDMAP_BASE" "$LXD_IDMAP_SIZE"; then
        err "This is an unprivileged LXC host without Remote's nested UID/GID allocation."
        cat >&2 <<EOF

  Remote project containers are intentionally unprivileged and map their root
  user to host UID/GID $LXD_IDMAP_BASE. The outer Proxmox container must expose
  the full $LXD_IDMAP_SIZE-ID range beginning at $LXD_IDMAP_BASE in both
  /proc/self/uid_map and /proc/self/gid_map.

  Configure that subordinate range and enable nesting in the Proxmox container
  settings, restart the container, then run this installer again. The installer
  will not silently use privileged project containers because that weakens the
  security boundary between agent workspaces and the host.
EOF
        exit 1
    fi
fi

if [ "$NESTED_UNPRIVILEGED_LXC" = "1" ] && [ "${ID:-}" = "debian" ]; then
    # Snap packages need SquashFS loop/FUSE mounts and access to the host
    # AppArmor security filesystem, which Proxmox does not expose to an
    # unprivileged LXC by default. Debian's native package avoids that
    # packaging-layer requirement while providing the same LXD API/CLI.
    if ! command -v lxc >/dev/null; then
        log "Installing native Debian LXD packages for nested LXC"
        apt-get install -y -qq lxd lxd-client
        systemctl enable --now lxd.socket
    fi
else
    # Checked via `snap list lxd`, not `command -v lxc`: Ubuntu 24.04 ships the
    # `lxd-installer` transitional package, which pre-populates /usr/bin/lxc and
    # /usr/sbin/lxd as shims that lazily install the snap on first invocation.
    # `command -v lxc` finds those shims on a completely fresh box, so this step
    # would silently skip the real install — and the first real LXD command
    # later in this script (or `lxd init --auto` below) would trigger the
    # shim's own uncontrolled auto-install instead, outside our retry/wait
    # logic and prone to leaving the host half-installed on any hiccup.
    if ! snap list lxd >/dev/null 2>&1; then
        log "Installing LXD via snap"
        if ! command -v snap >/dev/null; then
            apt-get install -y -qq snapd
            systemctl enable --now snapd.socket
            for _ in 1 2 3 4 5; do snap wait system seed.loaded && break; sleep 1; done
        fi
        snap install lxd
    fi
    export PATH="/snap/bin:$PATH"
fi

# Initialize storage + bridge on fresh installs. `lxc network show lxdbr0`
# is our "initialized" probe.
if ! lxc network show lxdbr0 >/dev/null 2>&1; then
    log "lxd init --auto"
    lxd init --auto
fi
if [ "$NESTED_UNPRIVILEGED_LXC" = "1" ]; then
    # Debian's native package takes its initial allocation from /etc/subuid;
    # pin the profile to the idmap Remote's host-file ownership code expects.
    lxc profile set default security.idmap.base "$LXD_IDMAP_BASE"
    ok "nested LXD uses unprivileged idmap ${LXD_IDMAP_BASE}-$((LXD_IDMAP_BASE + LXD_IDMAP_SIZE - 1))"
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
