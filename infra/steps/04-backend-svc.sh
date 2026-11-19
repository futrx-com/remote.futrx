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

ensure_root_linger() {
    # Snap's lxc client can attach to root's user manager during an SSH session.
    # Keep that manager alive after logout so in-flight lxc init calls survive.
    if ! command -v loginctl >/dev/null 2>&1; then
        warn "loginctl is unavailable; could not enable root linger for LXD commands."
        return 0
    fi
    if ! loginctl enable-linger root; then
        warn "Could not enable root linger; SSH logout may interrupt LXD commands."
    fi
}

step_04_backend_svc() {
ensure_root_linger
SERVICE_NAME="remote.futrx.service"
SERVICE_UNIT_PATH="/etc/systemd/system/$SERVICE_NAME"
LEGACY_SERVICE_NAME="remote.futrx.dev.service"
LEGACY_SERVICE_UNIT_PATH="/etc/systemd/system/$LEGACY_SERVICE_NAME"
HOST_CLI_PROFILE_PATH="/etc/profile.d/remote-futrx-host-clis.sh"

# shellcheck source=../lib/install-migration.sh
. "$INFRA_DIR/lib/install-migration.sh"
# shellcheck source=../lib/health-check.sh
. "$INFRA_DIR/lib/health-check.sh"
# shellcheck source=../lib/chat-migration.sh
. "$INFRA_DIR/lib/chat-migration.sh"

# A legacy service binary cannot read the new authoritative database. Keep
# rollback conservative after cutover, even if startup fails before health.
rollback_storage_compatible_legacy_service() {
    if [ -e "$INSTALL_DIR/data/conversations.sqlite" ]; then
        local storage_status
        storage_status="$(DATA_DIR="$INSTALL_DIR/data" "$INSTALL_DIR/backend/remote" storage-status 2>/dev/null)" || storage_status="unknown"
        case "$storage_status" in
            legacy|importing) ;;
            *)
                err "Conversation storage is SQLite or its status is uncertain; legacy service rollback is blocked. Recover with a SQLite-compatible release."
                systemctl stop "$SERVICE_NAME" || true
                return 1
                ;;
        esac
    fi
    rollback_legacy_service_migration "$SERVICE_NAME" "$LEGACY_SERVICE_NAME"
}

# Shell commands need the same installation settings as the service below.
# Some existing hosts do not have /usr/local/bin yet, and standard input is not
# a portable install(1) source. Render to a real file before installing it.
local remote_cli_source

log "Installing /usr/local/bin/remote"
install -d -o root -g root -m 0755 /usr/local/bin
remote_cli_source="$(mktemp "${TMPDIR:-/tmp}/remote-futrx-cli.XXXXXX")"
if ! {
    printf '#!/bin/bash\n'
    printf 'export BASE_URL=%q\n' "https://$HOSTNAME"
    printf 'export DATA_DIR=%q\n' "$INSTALL_DIR/data"
    printf 'export INSTALL_DIR=%q\n' "$INSTALL_DIR"
    printf 'exec %q "$@"\n' "$INSTALL_DIR/backend/remote"
} > "$remote_cli_source"; then
    rm -f -- "$remote_cli_source"
    err "Could not render the remote CLI launcher."
    return 1
fi
if ! install -o root -g root -m 0755 -T "$remote_cli_source" /usr/local/bin/remote; then
    rm -f -- "$remote_cli_source"
    err "Could not install /usr/local/bin/remote."
    return 1
fi
rm -f -- "$remote_cli_source"

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

# The backend must be fully stopped before reading the legacy source. This
# phase has its own lifetime; the 30-second HTTP deadline starts only below.
log "Migrating conversation storage"
if ! remote_migrate_conversations "$INSTALL_DIR/backend/remote" "$INSTALL_DIR/data" "$SERVICE_NAME"; then
    err "Conversation migration failed; service remains stopped. Fix the reported source issue and rerun the upgrade to resume."
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
    if ! rollback_storage_compatible_legacy_service; then
        err "Automatic rollback to $LEGACY_SERVICE_NAME also failed."
    fi
    journalctl -u "$SERVICE_NAME" -n 30 --no-pager >&2 || true
    exit 1
fi

# ───────────────── health check ─────────────────
if ! systemctl is-active --quiet "$SERVICE_NAME"; then
    err "Service failed to start. Recent logs:"
    if ! rollback_storage_compatible_legacy_service; then
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
    if ! rollback_storage_compatible_legacy_service; then
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
}
