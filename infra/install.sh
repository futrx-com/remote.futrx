#!/usr/bin/env bash
#
# remote.futrx — fresh installer and full host-convergence entry point.
#
# Use this for a first installation or an intentional repair/re-convergence of
# an existing host. Step 00 selects and re-executes the application checkout;
# the remaining infra/steps/*.sh converge host packages and pinned toolchains,
# apply that commit's host agent catalog, build the application, render
# Caddy and systemd configuration, ensure the LXD base image exists, harden
# SSH, and install the container-network repair timer.
#
# The steps are designed to be idempotent, but a re-run is not an
# application-only deployment: it can change host configuration and restart
# the backend. For normal releases, prefer Settings -> Updates. Manually use
# infra/deploy-app.sh for a same-major/minor application release or
# infra/update.sh for a major/minor infrastructure release.
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
#   --github-token=ghp_xxx                      private-repo PAT; prefer the
#                                               GITHUB_TOKEN environment variable
#                                               to avoid placing it in shell history.
#   --google-client-id=...                      optional; can be added in Settings later.
#   --google-client-secret=...                  optional; can be added in Settings later.
#
# Environment:
#   GITHUB_TOKEN                                same as --github-token.
#   FUTRX_INSTALL_DIR                           override /opt/remote.futrx (QA/tests).

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
    MAIN_REFSPEC="+refs/heads/main:refs/remotes/origin/main"

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
                # Existing shallow clones may follow a different default branch,
                # so fetch main explicitly instead of relying on their refspec.
                # Persist that refspec too — otherwise remote.origin.fetch stays
                # on the stale branch and a later plain `git fetch origin` (in
                # update.sh, steps/00-checkout.sh, ...) silently stops refreshing
                # origin/main.
                git -C "$LEGACY_TARGET" config remote.origin.fetch "$MAIN_REFSPEC"
                git -C "$LEGACY_TARGET" fetch --quiet --tags origin
                git -C "$LEGACY_TARGET" reset --hard origin/main
            fi
            export FUTRX_INSTALL_CHECKOUT_SELECTED=1
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
        # Production installs track main regardless of the repository's GitHub
        # default branch (which may temporarily point at a QA branch).
        git clone --depth=1 --branch main --single-branch "$CLONE_URL" "$TARGET"
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
            # Persist the main refspec (not just this one-off fetch) so later
            # plain `git fetch origin` calls — update.sh, steps/00-checkout.sh —
            # keep refreshing origin/main instead of silently going stale on a
            # checkout whose remote.origin.fetch still points at another branch.
            git -C "$TARGET" config remote.origin.fetch "$MAIN_REFSPEC"
            git -C "$TARGET" fetch --quiet --tags origin
            git -C "$TARGET" reset --hard origin/main
        fi
    fi

    export FUTRX_INSTALL_CHECKOUT_SELECTED=1
    exec bash "$TARGET/infra/install.sh" "$@"
fi

# ───────────────── args ─────────────────
HOSTNAME=""
SKIP_DNS_CHECK=0
GOOGLE_CLIENT_ID=""
GOOGLE_CLIENT_SECRET=""
GITHUB_TOKEN="${GITHUB_TOKEN:-}"
TARGET_REF=""
for a in "$@"; do
    case "$a" in
        --skip-dns-check)         SKIP_DNS_CHECK=1 ;;
        --ref=*)                  TARGET_REF="${a#*=}" ;;
        --google-client-id=*)     GOOGLE_CLIENT_ID="${a#*=}" ;;
        --google-client-secret=*) GOOGLE_CLIENT_SECRET="${a#*=}" ;;
        --github-token=*)         GITHUB_TOKEN="${a#*=}" ;;
        --*) echo "unknown flag: $a" >&2; exit 1 ;;
        *)   [ -z "$HOSTNAME" ] && HOSTNAME="$a" ;;
    esac
done
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
HOST_CLI_PREFIX="$INSTALL_DIR/data/host-clis"
HOST_CLI_BIN_DIR="$HOST_CLI_PREFIX/bin"

# Host agent installation and the backend must resolve the same executables.
# Use an application-owned prefix ahead of host-global locations so legacy or
# manually installed binaries cannot shadow Remote's pinned toolchain.
PATH="$HOST_CLI_BIN_DIR:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/snap/bin"

# Escape dots in HOSTNAME for Caddy regex (dots match any char in regex; we
# want literal matches).
HOSTNAME_RE="$(printf '%s' "$HOSTNAME" | sed 's/\./\\./g')"

export INFRA_DIR INSTALL_DIR LEGACY_INSTALL_DIR REPO_URL SERVICE_PORT HOSTNAME_RE
export HOST_CLI_PREFIX HOST_CLI_BIN_DIR PATH

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
    envsubst '$HOSTNAME $HOSTNAME_RE $INSTALL_DIR $SERVICE_PORT $LXD_BRIDGE_IP $LXD_BRIDGE $HOST_CLI_BIN_DIR' \
        < "$tmpl" > "$dest"
}
export -f render_template

# ───────────────── pre-rename installation migration ─────────────────
# shellcheck source=lib/install-migration.sh
. "$INFRA_DIR/lib/install-migration.sh"
migrate_legacy_install_dir "$INSTALL_DIR" "$LEGACY_INSTALL_DIR"
if [ "$FUTRX_INSTALL_PATH_MIGRATED" -eq 1 ]; then
    INFRA_DIR="$INSTALL_DIR/infra"
    export INFRA_DIR
fi

# ───────────────── select checkout and re-exec ─────────────────
# This precedes every version/catalog consumer so direct installer reruns are
# as commit-consistent as update.sh.
# shellcheck source=steps/00-checkout.sh
. "$INFRA_DIR/steps/00-checkout.sh"

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

# ───────────────── run the convergence steps ─────────────────
# shellcheck source=steps/01-host-deps.sh
. "$INFRA_DIR/steps/01-host-deps.sh"
# shellcheck source=steps/02-app.sh
. "$INFRA_DIR/steps/02-app.sh"
# shellcheck source=steps/03-caddy.sh
. "$INFRA_DIR/steps/03-caddy.sh"
# shellcheck source=steps/04-backend-svc.sh
. "$INFRA_DIR/steps/04-backend-svc.sh"
# shellcheck source=steps/05-base-image.sh
. "$INFRA_DIR/steps/05-base-image.sh"
# shellcheck source=steps/06-ssh-hardening.sh
. "$INFRA_DIR/steps/06-ssh-hardening.sh"
# shellcheck source=steps/07-lxc-ipv4-heal.sh
. "$INFRA_DIR/steps/07-lxc-ipv4-heal.sh"

# ───────────────── summary ─────────────────
cat <<EOF

═══════════════════════════════════════════════════════════════
 ✓ Installed at:  $INSTALL_DIR
 ✓ Main UI:       https://$HOSTNAME
 ✓ Code editor:   https://code.$HOSTNAME
 ✓ Dev URLs:      https://<slug>--<port>.dev.$HOSTNAME
 ✓ DB viewers:    lazy per project at https://<slug>--18080.dev.$HOSTNAME
 ✓ Base image:    futrx-remote-dev-base (project containers launch from this)

 $AUTH_NOTE

 Next:
   1. Open https://$HOSTNAME (Caddy fetches the cert on first hit, ~10s)
   2. Create the administrator email and password
   3. Finish setup for at least one access-gate coding agent
   4. Before inviting users, configure Google sign-in in Settings → Users

 If you're on a cloud VPS with its own firewall, open 80/443 in the
 provider's console as well as UFW.

 Manage:
   systemctl status   remote.futrx
   systemctl status   caddy
   journalctl -u      remote.futrx -f

 For future releases, use Settings → Updates so Remote selects the safe
 application or infrastructure path. Re-run this installer only for an
 intentional full host repair/re-convergence.
═══════════════════════════════════════════════════════════════

EOF
