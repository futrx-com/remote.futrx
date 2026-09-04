#!/usr/bin/env bash
# Render /etc/caddy/Caddyfile from templates/Caddyfile.tmpl and reload.
# Always overwrites — the file is generated, not hand-edited.
#
# Expects from caller:
#   - log / ok / err helpers
#   - $INFRA_DIR, $HOSTNAME, $HOSTNAME_RE, $SERVICE_PORT
set -euo pipefail

log "Rendering /etc/caddy/Caddyfile for $HOSTNAME"
# Render to a temp file first, validate, then atomically replace. This way a
# bad template doesn't blow away a working live config.
TMP_CADDYFILE="$(mktemp)"
trap 'rm -f "$TMP_CADDYFILE"' RETURN
render_template "${INFRA_DIR}/templates/Caddyfile.tmpl" "$TMP_CADDYFILE"

# Optional DNS-01: when no provider is configured, strip the acme_dns
# line so caddy validate does not fail on an empty directive. When a
# provider is configured, replace the system Caddy with a build that
# includes the matching DNS module so DNS-01 challenges actually work.
if [ -z "${ACME_DNS_PROVIDER:-}" ]; then
    sed -i '/^ *acme_dns[[:space:]]*$/d' "$TMP_CADDYFILE"
else
    log "Installing Caddy with DNS-01 provider module: $ACME_DNS_PROVIDER"
    CADDY_URL="https://caddyserver.com/api/download?p=${ACME_DNS_PROVIDER}&os=linux&arch=amd64"
    CADDY_TMP="$(mktemp)"
    CADDY_DIR="$(mktemp -d)"
    curl -fsSL --max-time 120 -o "$CADDY_TMP" "$CADDY_URL"
    mkdir -p "$CADDY_DIR"
    tar -xzf "$CADDY_TMP" -C "$CADDY_DIR"
    # The build tarball may contain the binary at the root or under a
    # leading folder (e.g. caddy/caddy). Locate it rather than assume.
    CADDY_BIN="$(find "$CADDY_DIR" -type f -name caddy -print -quit)"
    [ -n "$CADDY_BIN" ] || err "Could not find caddy binary in downloaded archive"
    install -m 0755 "$CADDY_BIN" /usr/bin/caddy
    rm -rf "$CADDY_TMP" "$CADDY_DIR"
    ok "Caddy updated with $ACME_DNS_PROVIDER DNS module ($(caddy version | head -1))"
fi

if ! caddy validate --config "$TMP_CADDYFILE" --adapter caddyfile >/dev/null 2>&1; then
    err "Rendered Caddyfile is invalid — leaving live config untouched."
    caddy validate --config "$TMP_CADDYFILE" --adapter caddyfile 2>&1 | tail -20 >&2
    echo "  (rendered preview at: $TMP_CADDYFILE)" >&2
    trap - RETURN
    exit 1
fi

# Only replace the live file if it actually changed — keeps `systemctl reload`
# a true no-op when there's nothing to do.
if ! cmp -s "$TMP_CADDYFILE" /etc/caddy/Caddyfile 2>/dev/null; then
    install -m 0644 "$TMP_CADDYFILE" /etc/caddy/Caddyfile
    CADDYFILE_CHANGED=1
else
    CADDYFILE_CHANGED=0
fi

systemctl enable caddy >/dev/null 2>&1 || true

if ! systemctl is-active --quiet caddy; then
    systemctl restart caddy
    ok "Caddy started"
elif [ "${CADDYFILE_CHANGED:-0}" = "1" ]; then
    systemctl reload caddy
    ok "Caddy reloaded (config changed)"
else
    ok "Caddy already running with current config"
fi
