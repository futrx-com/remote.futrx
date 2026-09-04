#!/usr/bin/env bash
# Backend systemd unit: render, enable, start (or restart on re-run).
# Includes a post-start health check that fails loudly if the binary doesn't
# respond on its loopback port.
#
# Also handles UFW (opens 80/443 if the firewall is active).
#
# Expects from caller:
#   - log / ok / err helpers
#   - $INFRA_DIR, $INSTALL_DIR, $HOSTNAME, $SERVICE_PORT
set -euo pipefail

SERVICE_NAME="remote.futrx.service"
SERVICE_UNIT_PATH="/etc/systemd/system/$SERVICE_NAME"
LEGACY_SERVICE_NAME="remote.futrx.dev.service"
LEGACY_SERVICE_UNIT_PATH="/etc/systemd/system/$LEGACY_SERVICE_NAME"
HOST_CLI_PROFILE_PATH="/etc/profile.d/remote-futrx-host-clis.sh"

# shellcheck source=../lib/install-migration.sh
. "$INFRA_DIR/lib/install-migration.sh"
# shellcheck source=../lib/health-check.sh
. "$INFRA_DIR/lib/health-check.sh"

# ───────────────── systemd unit ─────────────────
log "Rendering $HOST_CLI_PROFILE_PATH"
render_template "${INFRA_DIR}/templates/remote-futrx-host-clis.sh.tmpl" \
                "$HOST_CLI_PROFILE_PATH"
chmod 0644 "$HOST_CLI_PROFILE_PATH"

log "Rendering $SERVICE_UNIT_PATH"
render_template "${INFRA_DIR}/templates/remote.futrx.service.tmpl" \
                "$SERVICE_UNIT_PATH"
systemctl daemon-reload

if ! prepare_legacy_service_migration "$LEGACY_SERVICE_NAME" "$LEGACY_SERVICE_UNIT_PATH"; then
    err "Could not pause $LEGACY_SERVICE_NAME; the existing service was left in place."
    exit 1
fi

if systemctl is-active --quiet "$SERVICE_NAME"; then
    log "Restarting $SERVICE_NAME"
    SERVICE_ACTION="restart"
else
    log "Starting $SERVICE_NAME"
    SERVICE_ACTION="enable"
fi
if { [ "$SERVICE_ACTION" = "restart" ] && ! systemctl restart "$SERVICE_NAME"; } || \
   { [ "$SERVICE_ACTION" = "enable" ] && ! systemctl enable --now "$SERVICE_NAME"; }; then
    err "$SERVICE_NAME failed to $SERVICE_ACTION."
    if ! rollback_legacy_service_migration "$SERVICE_NAME" "$LEGACY_SERVICE_NAME"; then
        err "Automatic rollback to $LEGACY_SERVICE_NAME also failed."
    fi
    journalctl -u "$SERVICE_NAME" -n 30 --no-pager >&2 || true
    exit 1
fi

# ───────────────── health check ─────────────────
if ! systemctl is-active --quiet "$SERVICE_NAME"; then
    err "Service failed to start. Recent logs:"
    if ! rollback_legacy_service_migration "$SERVICE_NAME" "$LEGACY_SERVICE_NAME"; then
        err "Automatic rollback to $LEGACY_SERVICE_NAME also failed."
    fi
    journalctl -u "$SERVICE_NAME" -n 30 --no-pager >&2 || true
    exit 1
fi

log "Health-checking backend on 127.0.0.1:${SERVICE_PORT}"
# Cold start binds the port only after converging container resource envelopes
# (~13s with 16 projects), so allow a real 30-second wall-clock deadline.
if ! wait_for_http_health "http://127.0.0.1:${SERVICE_PORT}/" 30; then
    err "Backend did not respond on 127.0.0.1:${SERVICE_PORT} within 30s"
    if ! rollback_legacy_service_migration "$SERVICE_NAME" "$LEGACY_SERVICE_NAME"; then
        err "Automatic rollback to $LEGACY_SERVICE_NAME also failed."
    fi
    journalctl -u "$SERVICE_NAME" -n 30 --no-pager >&2 || true
    exit 1
fi
ok "backend responding"

if ! complete_legacy_service_migration "$LEGACY_SERVICE_NAME" "$LEGACY_SERVICE_UNIT_PATH"; then
    err "Backend is healthy, but cleanup of $LEGACY_SERVICE_NAME failed."
    exit 1
fi

# ───────────────── UFW ─────────────────
if command -v ufw >/dev/null && ufw status 2>/dev/null | grep -q "Status: active"; then
    log "Opening UFW for 80 + 443"
    ufw allow 80/tcp  >/dev/null || true
    ufw allow 443/tcp >/dev/null || true
fi
