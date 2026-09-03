#!/usr/bin/env bash
# Render /etc/caddy/Caddyfile from templates/Caddyfile.tmpl and reload.
# Always overwrites — the file is generated, not hand-edited.
#
# Expects from caller:
#   - log / ok / err helpers
#   - $INFRA_DIR, $HOSTNAME, $HOSTNAME_RE, $SERVICE_PORT
set -euo pipefail

log "Rendering /etc/caddy/Caddyfile for $HOSTNAME"
# Ensure user snippet directories exist so Caddy can safely import them.
mkdir -p /etc/caddy/conf.d/global /etc/caddy/conf.d/sites
chmod 0755 /etc/caddy/conf.d /etc/caddy/conf.d/global /etc/caddy/conf.d/sites 2>/dev/null || true

# Render to a temp file first, validate, then atomically replace. This way a
# bad template doesn't blow away a working live config.
TMP_CADDYFILE="$(mktemp)"
trap 'rm -f "$TMP_CADDYFILE"' RETURN
render_template "${INFRA_DIR}/templates/Caddyfile.tmpl" "$TMP_CADDYFILE"

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
