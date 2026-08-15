#!/usr/bin/env bash
#
# remote.futrx — main installer / deployer.
# Walks through infra/steps/*.sh in order. Idempotent: safe to re-run on
# every CI deploy as well as the initial bootstrap.
#
# Usage (from a clone):
#   sudo bash infra/install.sh <hostname> [flags]
#
# Usage (fresh box, curl|bash):
#   curl -fsSL https://remote.futrx.com/get | sudo bash -s -- <hostname> [flags]
#   (https://remote.futrx.com/get is a 301 to this file's raw.githubusercontent
#   URL, served by the marketing site's public/_redirects.)
#
# Flags:
#   --skip-dns-check                            useful on cloud bootstrap where the public
#                                               A record is set but propagation isn't done.
#   --ref=<40-character commit SHA>             install an immutable candidate commit instead
#                                               of origin/main (used by QA branch installs).
#   --github-token=ghp_xxx                      private-repo PAT.
#   --google-client-id=...                      optional; can be added in Settings later.
#   --google-client-secret=...                  optional; can be added in Settings later.
#
# Plesk hosts (auto-detected; these only apply when Plesk is present):
#   --no-plesk-integration                      install as if Plesk were not here. Will fail
#                                               the 80/443 pre-flight unless you free them.
#   --plesk-integration                         undo the above (these choices are remembered
#                                               across re-runs and updates).
#   --plesk-tls-cert=/path --plesk-tls-key=...  certificate for Plesk's nginx to serve. Must
#                                               cover the wildcards; auto-detected from
#                                               Plesk's own store when omitted.
#   --caddy-port=8080                           loopback port Plesk's nginx proxies to.
#   --force-ssh-hardening                       disable SSH password auth anyway. Skipped by
#                                               default on Plesk, where it can lock out
#                                               panel-managed SFTP users.
#   --no-force-ssh-hardening                    undo the above.
#
# The Plesk choices persist to /etc/remote.futrx/install-options.env, so
# infra/update.sh does not silently change topology. Delete that file to reset.
#
# Environment:
#   GITHUB_TOKEN                                same as --github-token=.

set -euo pipefail

# ───────────────── self-bootstrap (curl|bash mode) ─────────────────
# When piped from curl, BASH_SOURCE points at /dev/stdin and there are no
# sibling steps/ or templates/. Install git, clone the repo to the canonical
# install dir, and re-exec from there so the rest of this script sees real
# files on disk.
INFRA_DIR_PROBE="$( cd "$( dirname "${BASH_SOURCE[0]:-}" 2>/dev/null || echo . )" >/dev/null 2>&1 && pwd || true )"
if [ -z "$INFRA_DIR_PROBE" ] || [ ! -d "${INFRA_DIR_PROBE}/steps" ]; then
    if [ "$EUID" -ne 0 ]; then
        echo "this installer needs root; rerun with sudo" >&2
        exit 1
    fi
    export DEBIAN_FRONTEND=noninteractive
    if ! command -v git >/dev/null; then
        apt-get update -qq
        apt-get install -y -qq git ca-certificates
    fi

    TARGET="${FUTRX_INSTALL_DIR:-/opt/remote.futrx}"
    LEGACY_TARGET="${FUTRX_LEGACY_INSTALL_DIR:-/opt/remote.futrx.dev}"

    # Parse bootstrap-only values before touching either checkout. The full
    # argument loop below parses them again after we re-exec from disk.
    BOOTSTRAP_TOKEN="${GITHUB_TOKEN:-}"
    BOOTSTRAP_REF=""
    for a in "$@"; do
        case "$a" in
            --github-token=*) BOOTSTRAP_TOKEN="${a#*=}" ;;
            --ref=*)          BOOTSTRAP_REF="${a#*=}" ;;
        esac
    done
    if [ -n "$BOOTSTRAP_REF" ] && ! printf '%s' "$BOOTSTRAP_REF" | grep -qE '^[0-9a-fA-F]{40}$'; then
        echo "--ref must be a full 40-character commit SHA" >&2
        exit 1
    fi

    # A pre-rename checkout can update itself in place, then the checked-out
    # installer below performs the guarded path migration with rollback.
    if [ -d "$LEGACY_TARGET/.git" ] && [ ! -L "$LEGACY_TARGET" ]; then
        if [ -d "$TARGET/.git" ]; then
            echo "both $LEGACY_TARGET and $TARGET contain installations; refusing to overwrite either" >&2
            exit 1
        fi
        if [ ! -e "$TARGET" ] || { [ -d "$TARGET" ] && [ -z "$(ls -A "$TARGET" 2>/dev/null)" ]; }; then
            echo "==> bootstrapping from legacy checkout at $LEGACY_TARGET"
            if [ -n "$BOOTSTRAP_REF" ]; then
                git -C "$LEGACY_TARGET" fetch --quiet --depth=1 origin "$BOOTSTRAP_REF"
                git -C "$LEGACY_TARGET" reset --hard "$BOOTSTRAP_REF"
            else
                git -C "$LEGACY_TARGET" fetch --quiet --tags origin
                git -C "$LEGACY_TARGET" reset --hard origin/main
            fi
            exec bash "$LEGACY_TARGET/infra/install.sh" "$@"
        fi
    fi

    # Honor --github-token / GITHUB_TOKEN for the bootstrap clone of a private
    # repo.
    CLONE_URL="https://github.com/futrx-com/remote.futrx.git"
    if [ -n "$BOOTSTRAP_TOKEN" ]; then
        CLONE_URL="https://x-access-token:${BOOTSTRAP_TOKEN}@github.com/futrx-com/remote.futrx.git"
    fi

    if [ ! -d "$TARGET/.git" ]; then
        echo "==> bootstrapping: cloning repo to $TARGET"
        if [ -d "$TARGET" ] && [ -n "$(ls -A "$TARGET" 2>/dev/null)" ]; then
            echo "$TARGET exists and is not empty — refusing to overwrite" >&2
            exit 1
        fi
        mkdir -p "$TARGET"
        git clone --depth=1 "$CLONE_URL" "$TARGET"
        chmod 0600 "$TARGET/.git/config"
        if [ -n "$BOOTSTRAP_REF" ]; then
            echo "==> bootstrapping candidate commit $BOOTSTRAP_REF"
            git -C "$TARGET" fetch --quiet --depth=1 origin "$BOOTSTRAP_REF"
            git -C "$TARGET" reset --hard "$BOOTSTRAP_REF"
        fi
    else
        if [ -n "$BOOTSTRAP_REF" ]; then
            echo "==> repo already at $TARGET, selecting candidate commit $BOOTSTRAP_REF"
            git -C "$TARGET" fetch --quiet --depth=1 origin "$BOOTSTRAP_REF"
            git -C "$TARGET" reset --hard "$BOOTSTRAP_REF"
        else
            echo "==> repo already at $TARGET, pulling latest"
            git -C "$TARGET" fetch --quiet --tags origin
            git -C "$TARGET" reset --hard origin/main
        fi
    fi

    exec bash "$TARGET/infra/install.sh" "$@"
fi

# ───────────────── args ─────────────────
HOSTNAME=""
SKIP_DNS_CHECK=0
GOOGLE_CLIENT_ID=""
GOOGLE_CLIENT_SECRET=""
GITHUB_TOKEN="${GITHUB_TOKEN:-}"
TARGET_REF=""
NO_PLESK_INTEGRATION=""
FORCE_SSH_HARDENING=""
PLESK_TLS_CERT=""
PLESK_TLS_KEY=""
CADDY_HTTP_PORT="${CADDY_HTTP_PORT:-}"
for a in "$@"; do
    case "$a" in
        --skip-dns-check)         SKIP_DNS_CHECK=1 ;;
        --ref=*)                  TARGET_REF="${a#*=}" ;;
        --google-client-id=*)     GOOGLE_CLIENT_ID="${a#*=}" ;;
        --google-client-secret=*) GOOGLE_CLIENT_SECRET="${a#*=}" ;;
        --github-token=*)         GITHUB_TOKEN="${a#*=}" ;;
        --no-plesk-integration)   NO_PLESK_INTEGRATION=1 ;;
        --plesk-integration)      NO_PLESK_INTEGRATION=0 ;;
        --force-ssh-hardening)    FORCE_SSH_HARDENING=1 ;;
        --no-force-ssh-hardening) FORCE_SSH_HARDENING=0 ;;
        --plesk-tls-cert=*)       PLESK_TLS_CERT="${a#*=}" ;;
        --plesk-tls-key=*)        PLESK_TLS_KEY="${a#*=}" ;;
        --caddy-port=*)           CADDY_HTTP_PORT="${a#*=}" ;;
        --*) echo "unknown flag: $a" >&2; exit 1 ;;
        *)   [ -z "$HOSTNAME" ] && HOSTNAME="$a" ;;
    esac
done
if { [ -n "$PLESK_TLS_CERT" ] && [ -z "$PLESK_TLS_KEY" ]; } || \
   { [ -z "$PLESK_TLS_CERT" ] && [ -n "$PLESK_TLS_KEY" ]; }; then
    echo "--plesk-tls-cert and --plesk-tls-key must be given together" >&2
    exit 1
fi
if [ -n "$TARGET_REF" ] && ! printf '%s' "$TARGET_REF" | grep -qE '^[0-9a-fA-F]{40}$'; then
    echo "--ref must be a full 40-character commit SHA" >&2
    exit 1
fi
if [ -z "$HOSTNAME" ]; then
    read -rp "Public hostname (must already point here in DNS): " HOSTNAME || true
fi
if [ -z "$HOSTNAME" ]; then
    echo "hostname is required (pass as first argument)" >&2
    echo "  example: sudo bash infra/install.sh remote.example.com" >&2
    exit 1
fi
# Refuse an IP literal — Let's Encrypt rejects IP identifiers, and rendering
# the Caddyfile with an IP as the hostname produces a config Caddy still
# loads but can't get a cert for, taking the site down silently. A short
# regex catches both IPv4 and bracketed-IPv6 attempts.
if printf '%s' "$HOSTNAME" | grep -qE '^([0-9]{1,3}\.){3}[0-9]{1,3}$|^\[.*\]$'; then
    echo "hostname must be a DNS name, not an IP address (got: $HOSTNAME)" >&2
    echo "  Let's Encrypt cannot issue certs for IPs and your site will lose TLS." >&2
    exit 1
fi
if [ "$EUID" -ne 0 ]; then
    echo "this installer needs root; rerun with sudo" >&2
    exit 1
fi

export HOSTNAME GITHUB_TOKEN GOOGLE_CLIENT_ID GOOGLE_CLIENT_SECRET
if [ -n "$TARGET_REF" ]; then
    export FUTRX_CHECKOUT_REF="$TARGET_REF"
fi

# ───────────────── globals ─────────────────
INFRA_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
INSTALL_DIR="${FUTRX_INSTALL_DIR:-/opt/remote.futrx}"
LEGACY_INSTALL_DIR="${FUTRX_LEGACY_INSTALL_DIR:-/opt/remote.futrx.dev}"
REPO_URL="https://github.com/futrx-com/remote.futrx.git"
SERVICE_PORT="${SERVICE_PORT:-7682}"

# Escape dots in HOSTNAME for Caddy regex (dots match any char in regex; we
# want literal matches).
HOSTNAME_RE="$(printf '%s' "$HOSTNAME" | sed 's/\./\\./g')"

export INFRA_DIR INSTALL_DIR LEGACY_INSTALL_DIR REPO_URL SERVICE_PORT HOSTNAME_RE

# ───────────────── helpers (sourced by steps) ─────────────────
log()  { printf "\n\033[1;36m==> %s\033[0m\n" "$*"; }
warn() { printf "\n\033[1;33m!! %s\033[0m\n" "$*"; }
ok()   { printf "\033[1;32m✓\033[0m %s\n" "$*"; }
err()  { printf "\n\033[1;31m✗ %s\033[0m\n" "$*" >&2; }
export -f log warn ok err

# render_template TEMPLATE_PATH DEST_PATH
# Whitelisted envsubst — only the variables we name get substituted, so
# stray $-prefixed strings in the template (e.g. Caddy's `{re.host.1}`,
# regex `\$` anchors) survive untouched.
render_template() {
    local tmpl="$1" dest="$2"
    envsubst '$HOSTNAME $HOSTNAME_RE $INSTALL_DIR $SERVICE_PORT $LXD_BRIDGE_IP $LXD_BRIDGE
              $CADDY_SITE_SCHEME $CADDY_SITE_PORT $CADDY_TLS_BLOCK $CADDY_GLOBAL_EXTRA
              $CADDY_HTTP_PORT $PLESK_LISTEN $PLESK_TLS $PLESK_HTTP_SERVER' \
        < "$tmpl" > "$dest"
}
export -f render_template

# ───────────────── front-end topology ─────────────────
# A Plesk box is not a fresh server: its nginx owns 80 and 443, its BIND owns
# 53, its firewall module reconciles iptables on its own schedule, and its
# SFTP users depend on SSH password auth. Decide once which topology we are
# installing, and let every step below branch on FUTRX_FRONTEND_MODE.
#
#   standalone  Caddy owns 80/443 and manages its own certificates. Unchanged.
#   plesk       Plesk's nginx terminates TLS and proxies to Caddy on loopback.
#
# shellcheck source=lib/plesk.sh
. "$INFRA_DIR/lib/plesk.sh"
# shellcheck source=lib/frontend-mode.sh
. "$INFRA_DIR/lib/frontend-mode.sh"
# shellcheck source=lib/install-options.sh
. "$INFRA_DIR/lib/install-options.sh"

# infra/update.sh re-invokes this script with only the hostname, so anything
# the operator chose the first time has to come back from disk or it silently
# reverts on the next update. An explicit flag still wins.
load_install_options
NO_PLESK_INTEGRATION="${NO_PLESK_INTEGRATION:-${FUTRX_SAVED_NO_PLESK_INTEGRATION:-0}}"
FORCE_SSH_HARDENING="${FORCE_SSH_HARDENING:-${FUTRX_SAVED_FORCE_SSH_HARDENING:-0}}"
CADDY_HTTP_PORT="${CADDY_HTTP_PORT:-${FUTRX_SAVED_CADDY_HTTP_PORT:-8080}}"

# Validate after resolution, not at parse time: these values can also arrive
# from the saved options file, and a hand-edited one would otherwise reach an
# arithmetic test or a rendered config unchecked.
[ "$NO_PLESK_INTEGRATION" = "1" ] || NO_PLESK_INTEGRATION=0
[ "$FORCE_SSH_HARDENING" = "1" ] || FORCE_SSH_HARDENING=0
if ! printf '%s' "$CADDY_HTTP_PORT" | grep -qE '^[0-9]{2,5}$'; then
    echo "--caddy-port must be a port number (got: $CADDY_HTTP_PORT)" >&2
    exit 1
fi

FUTRX_FRONTEND_MODE="$(select_frontend_mode "$NO_PLESK_INTEGRATION")"
set_caddy_render_vars "$FUTRX_FRONTEND_MODE" "$SERVICE_PORT" "$CADDY_HTTP_PORT"

export FUTRX_FRONTEND_MODE NO_PLESK_INTEGRATION FORCE_SSH_HARDENING
export CADDY_HTTP_PORT PLESK_TLS_CERT PLESK_TLS_KEY

save_install_options "$NO_PLESK_INTEGRATION" "$CADDY_HTTP_PORT" "$FORCE_SSH_HARDENING"

# ───────────────── pre-rename installation migration ─────────────────
# shellcheck source=lib/install-migration.sh
. "$INFRA_DIR/lib/install-migration.sh"
migrate_legacy_install_dir "$INSTALL_DIR" "$LEGACY_INSTALL_DIR"
if [ "$FUTRX_INSTALL_PATH_MIGRATED" -eq 1 ]; then
    INFRA_DIR="$INSTALL_DIR/infra"
    export INFRA_DIR
fi

# ───────────────── optional Google user authentication ─────────────────
# The administrator always claims the server with a local email/password.
# Google OAuth is only for invited users and may be configured later in the UI.
if { [ -n "$GOOGLE_CLIENT_ID" ] && [ -z "$GOOGLE_CLIENT_SECRET" ]; } || \
   { [ -z "$GOOGLE_CLIENT_ID" ] && [ -n "$GOOGLE_CLIENT_SECRET" ]; }; then
    err "Google OAuth requires both a client ID and a client secret."
    exit 1
fi

# ───────────────── distro guard ─────────────────
if [ ! -r /etc/os-release ]; then
    echo "cannot detect distro — /etc/os-release missing" >&2
    exit 1
fi
# shellcheck source=/dev/null
. /etc/os-release
case "${ID:-}" in
    ubuntu|debian) ;;
    *)
        echo "only Ubuntu/Debian supported (detected: ${ID:-unknown})." >&2
        exit 1
        ;;
esac

# ───────────────── DNS sanity check ─────────────────
# Best-effort verification that the hostname resolves to this server.
# Caddy's ACME challenge fails without correct DNS; we'd rather fail here
# than after spending 30s building.

# shellcheck source=lib/dns-resolve.sh
. "$INFRA_DIR/lib/dns-resolve.sh"

if [ "$SKIP_DNS_CHECK" -eq 0 ]; then
    log "Checking DNS: $HOSTNAME"
    SERVER_IP=""
    for endpoint in https://api.ipify.org https://ifconfig.me https://ipv4.icanhazip.com; do
        SERVER_IP=$(curl -4 -fsSL --max-time 5 "$endpoint" 2>/dev/null | tr -d '[:space:]' || true)
        [ -n "$SERVER_IP" ] && break
    done
    if [ -z "$SERVER_IP" ]; then
        warn "Could not detect this server's public IPv4 — skipping DNS check."
    else
        HOSTNAME_IPS=$(resolve_public_a "$HOSTNAME" || true)
        if [ -z "$HOSTNAME_IPS" ]; then
            err "$HOSTNAME does not resolve in public DNS."
            echo "  Server's public IPv4: $SERVER_IP" >&2
            echo "  Add an A record for $HOSTNAME → $SERVER_IP and wait for propagation." >&2
            echo "  Or re-run with --skip-dns-check (Cloudflare proxy / tunnels / etc)." >&2
            exit 1
        elif ! echo "$HOSTNAME_IPS" | grep -qx "$SERVER_IP"; then
            err "$HOSTNAME resolves to $(echo "$HOSTNAME_IPS" | paste -sd, -), not $SERVER_IP."
            echo "  Update the A record or re-run with --skip-dns-check." >&2
            exit 1
        fi
        ok "$HOSTNAME → $SERVER_IP (matches this server)"
    fi
fi

# ───────────────── run the steps ─────────────────
# shellcheck source=steps/01-host-deps.sh
. "$INFRA_DIR/steps/01-host-deps.sh"
# shellcheck source=steps/02-app.sh
. "$INFRA_DIR/steps/02-app.sh"
# shellcheck source=steps/03-caddy.sh
. "$INFRA_DIR/steps/03-caddy.sh"
# Plesk's nginx in front of Caddy. No-op on every other host. Runs straight
# after Caddy so the upstream it proxies to already exists and is valid.
# shellcheck source=steps/08-plesk-frontend.sh
. "$INFRA_DIR/steps/08-plesk-frontend.sh"
# shellcheck source=steps/04-backend-svc.sh
. "$INFRA_DIR/steps/04-backend-svc.sh"
# shellcheck source=steps/05-base-image.sh
. "$INFRA_DIR/steps/05-base-image.sh"
# shellcheck source=steps/06-ssh-hardening.sh
. "$INFRA_DIR/steps/06-ssh-hardening.sh"
# shellcheck source=steps/07-lxc-ipv4-heal.sh
. "$INFRA_DIR/steps/07-lxc-ipv4-heal.sh"

# ───────────────── summary ─────────────────
if [ "$FUTRX_FRONTEND_MODE" = "plesk" ]; then
    TOPOLOGY=" ✓ Front end:     Plesk nginx :443 → Caddy 127.0.0.1:${CADDY_HTTP_PORT} → backend
 ✓ nginx config:  /etc/nginx/conf.d/futrx.conf (managed here; re-run to update)"
    FIREWALL_NOTE=" Plesk manages this server's firewall, so UFW was left alone. If your
 provider has its own firewall, 80/443 must be open there."
    FIRST_HIT_NOTE="Plesk serves the certificate"
else
    TOPOLOGY=" ✓ Front end:     Caddy :80/:443 → backend"
    FIREWALL_NOTE=" If you're on a cloud VPS with its own firewall, open 80/443 in the
 provider's console as well as UFW."
    FIRST_HIT_NOTE="Caddy fetches the cert on first hit, ~10s"
fi

cat <<EOF

═══════════════════════════════════════════════════════════════
 ✓ Installed at:  $INSTALL_DIR
 ✓ Main UI:       https://$HOSTNAME
 ✓ Code editor:   https://code.$HOSTNAME
 ✓ Dev URLs:      https://<slug>--<port>.dev.$HOSTNAME
 ✓ DB viewers:    lazy per project at https://<slug>--18080.dev.$HOSTNAME
 ✓ Base image:    futrx-remote-dev-base (project containers launch from this)
$TOPOLOGY

 $AUTH_NOTE

 Next:
   1. Open https://$HOSTNAME ($FIRST_HIT_NOTE)
   2. Create the administrator email and password
   3. Connect at least one AI provider: Claude, Codex, or Kimi
   4. Before inviting users, configure Google sign-in in Settings → Users

$FIREWALL_NOTE

 Manage:
   systemctl status   remote.futrx
   systemctl status   caddy
   journalctl -u      remote.futrx -f

 If containers can't reach the internet:
   sudo bash $INSTALL_DIR/infra/diagnose-network.sh

 Re-run this installer any time to pull latest + rebuild + restart.
═══════════════════════════════════════════════════════════════

EOF
