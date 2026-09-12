#!/usr/bin/env bash
# Install the backup/restore tooling and a nightly timer.
#
# remote-backup snapshots DATA_DIR (backend quiesced for a few seconds),
# the project workspaces, provider tokens and host config into
# /var/backups/remote/<timestamp>/ with checksums, applies retention, and
# optionally copies the snapshot offsite with rclone. remote-restore puts a
# snapshot back. Both read /etc/remote-backup.env. See infra/backup/*.sh.
#
# Expects from caller: log / ok helpers, $INFRA_DIR.
set -euo pipefail

log "Installing remote-backup / remote-restore + nightly timer"

apt-get install -y -qq zstd >/dev/null 2>&1 || true

install -m 0755 "$INFRA_DIR/backup/remote-backup.sh"  /usr/local/sbin/remote-backup
install -m 0755 "$INFRA_DIR/backup/remote-restore.sh" /usr/local/sbin/remote-restore
install -m 0644 "$INFRA_DIR/templates/remote-backup.service.tmpl" /etc/systemd/system/remote-backup.service
install -m 0644 "$INFRA_DIR/templates/remote-backup.timer.tmpl"   /etc/systemd/system/remote-backup.timer

if [ ! -f /etc/remote-backup.env ]; then
  cat > /etc/remote-backup.env <<'ENV'
# remote-backup configuration (all optional). See /usr/local/sbin/remote-backup --help
#BACKUP_ROOT=/var/backups/remote
#KEEP_DAILY=7
#KEEP_WEEKLY=4
#WITH_CONTAINERS=0
#STOP_BACKEND=1
# Offsite copy target (run `rclone config` as root first), e.g. b2:bucket/remote or s3:bucket/remote
#RCLONE_TARGET=
ENV
  chmod 600 /etc/remote-backup.env
fi

mkdir -p /var/backups/remote
chmod 700 /var/backups/remote

systemctl daemon-reload
systemctl enable --now remote-backup.timer >/dev/null 2>&1

ok "remote-backup.timer active (nightly 03:30 UTC; 'remote-backup' any time, 'remote-restore <dir>' to restore)"
