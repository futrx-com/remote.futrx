#!/usr/bin/env bash
# remote-backup — snapshot everything Remote needs to be restored on a fresh host.
#
# What is captured (in this order):
#   1. control-plane state  DATA_DIR (users, projects, chats, secrets, session key,
#                           scheduled tasks, ...)   — taken with the backend STOPPED
#                           for a few seconds so no JSON/JSONL is torn mid-write.
#                           (KillMode=process: project containers, previews and IDEs
#                           keep running while the backend is down.)
#   2. workspaces           /var/lib/remote/projects/<slug>/{workspace,agent-home}
#                           (bind-mounted into containers)   — taken live.
#   3. host config          provider tokens (/root/.claude*, .codex, .kimi-code),
#                           /etc/caddy/Caddyfile, systemd unit(s), LXD profile dump.
#   4. (optional)           `lxc export` of every project container (--with-containers).
#                           Containers are cattle; this is only for hosts that installed
#                           things outside /workspace and cannot afford a re-provision.
#
# Output:  $BACKUP_ROOT/<UTC timestamp>/  with one tar.zst (or tar.gz) per part,
#          a manifest.json, and SHA256SUMS. Retention keeps the newest $KEEP_DAILY
#          plus one per ISO week for $KEEP_WEEKLY weeks. If $RCLONE_TARGET is set,
#          the finished snapshot dir is copied there (rclone must be installed and
#          configured for root).
#
# Config:  /etc/remote-backup.env  (KEY=VALUE, all optional):
#   BACKUP_ROOT=/var/backups/remote   KEEP_DAILY=7   KEEP_WEEKLY=4
#   RCLONE_TARGET=b2:my-bucket/remote   WITH_CONTAINERS=0   STOP_BACKEND=1
#   DATA_DIR=/opt/remote.futrx/data     PROJECTS_DIR=/var/lib/remote/projects
#
# Usage:   remote-backup [--with-containers] [--no-stop] [--dest DIR]
# Restore: remote-restore <snapshot-dir>       (installed alongside)
set -euo pipefail

CONF=/etc/remote-backup.env
[ -f "$CONF" ] && . "$CONF"

BACKUP_ROOT="${BACKUP_ROOT:-/var/backups/remote}"
KEEP_DAILY="${KEEP_DAILY:-7}"
KEEP_WEEKLY="${KEEP_WEEKLY:-4}"
RCLONE_TARGET="${RCLONE_TARGET:-}"
WITH_CONTAINERS="${WITH_CONTAINERS:-0}"
STOP_BACKEND="${STOP_BACKEND:-1}"
DATA_DIR="${DATA_DIR:-/opt/remote.futrx/data}"
PROJECTS_DIR="${PROJECTS_DIR:-/var/lib/remote/projects}"
SERVICE="${SERVICE:-remote.futrx}"

expect_dest=0
for a in "$@"; do
  if [ "$expect_dest" = 1 ]; then BACKUP_ROOT="$a"; expect_dest=0; continue; fi
  case "$a" in
    --with-containers) WITH_CONTAINERS=1 ;;
    --no-stop)         STOP_BACKEND=0 ;;
    --dest)            expect_dest=1 ;;
    -h|--help)         sed -n '2,30p' "$0"; exit 0 ;;
    *) echo "unknown arg: $a" >&2; exit 2 ;;
  esac
done

[ "$(id -u)" = 0 ] || { echo "must run as root" >&2; exit 1; }

if command -v zstd >/dev/null 2>&1; then TAR_Z="--zstd"; EXT="tar.zst"; else TAR_Z="-z"; EXT="tar.gz"; fi
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
DEST="$BACKUP_ROOT/$STAMP"
TMP="$DEST.partial"
log() { logger -t remote-backup -- "$*"; echo "remote-backup: $*" >&2; }
rm -rf "$TMP"; mkdir -p "$TMP"; chmod 700 "$BACKUP_ROOT" "$TMP"

started_at=$(date -u +%FT%TZ)
log "starting snapshot $STAMP -> $DEST"

# ── 1. control-plane state (quiesced) ────────────────────────────────────────
backend_was_active=0
restart_backend() {
  if [ "$backend_was_active" = 1 ]; then
    systemctl start "$SERVICE" || log "WARNING: failed to restart $SERVICE"
    backend_was_active=0
    log "backend restarted"
  fi
}
if [ "$STOP_BACKEND" = 1 ] && systemctl is-active --quiet "$SERVICE"; then
  backend_was_active=1
  systemctl stop "$SERVICE"
  log "backend stopped for consistent DATA_DIR snapshot"
fi
trap restart_backend EXIT

if [ -d "$DATA_DIR" ]; then
  tar $TAR_Z -cf "$TMP/data.$EXT" -C "$(dirname "$DATA_DIR")" "$(basename "$DATA_DIR")"
else
  log "WARNING: DATA_DIR $DATA_DIR missing"
fi
restart_backend
trap - EXIT

# ── 2. workspaces (live) ─────────────────────────────────────────────────────
if [ -d "$PROJECTS_DIR" ] && [ -n "$(ls -A "$PROJECTS_DIR" 2>/dev/null)" ]; then
  # agents may create/delete files mid-run; tar exit 1 (= "file changed") is acceptable
  set +e
  tar $TAR_Z --ignore-failed-read --warning=no-file-changed --warning=no-file-removed \
      -cf "$TMP/projects.$EXT" -C "$(dirname "$PROJECTS_DIR")" "$(basename "$PROJECTS_DIR")"
  rc=$?
  set -e
  if [ $rc -gt 1 ]; then log "ERROR: workspace archive failed (tar rc=$rc)"; rm -rf "$TMP"; exit 1; fi
fi

# ── 3. host config ───────────────────────────────────────────────────────────
HOSTCFG="$TMP/hostcfg"
mkdir -p "$HOSTCFG/root" "$HOSTCFG/etc/caddy" "$HOSTCFG/etc/systemd/system" "$HOSTCFG/lxd"
for d in /root/.claude /root/.claude.json /root/.codex /root/.kimi-code /root/.gemini; do
  if [ -e "$d" ]; then cp -a "$d" "$HOSTCFG/root/" 2>/dev/null || true; fi
done
[ -f /etc/caddy/Caddyfile ] && cp -a /etc/caddy/Caddyfile "$HOSTCFG/etc/caddy/"
for u in remote.futrx.service futrx-lxd-forward.service lxc-ipv4-heal.service lxc-ipv4-heal.timer remote-backup.service remote-backup.timer; do
  [ -f "/etc/systemd/system/$u" ] && cp -a "/etc/systemd/system/$u" "$HOSTCFG/etc/systemd/system/"
done
[ -f "$CONF" ] && cp -a "$CONF" "$HOSTCFG/etc/"
if command -v lxc >/dev/null 2>&1; then
  lxc profile show futrx-workspace > "$HOSTCFG/lxd/profile-futrx-workspace.yaml" 2>/dev/null || true
  lxc list --format json > "$HOSTCFG/lxd/containers.json" 2>/dev/null || true
  lxc config show > "$HOSTCFG/lxd/server-config.yaml" 2>/dev/null || true
fi
tar $TAR_Z -cf "$TMP/hostcfg.$EXT" -C "$TMP" hostcfg && rm -rf "$HOSTCFG"

# ── 4. optional container exports ────────────────────────────────────────────
if [ "$WITH_CONTAINERS" = 1 ] && command -v lxc >/dev/null 2>&1; then
  mkdir -p "$TMP/containers"
  while IFS=, read -r name; do
    [ -n "$name" ] || continue
    log "exporting container $name"
    lxc export "$name" "$TMP/containers/$name.tar.gz" --instance-only >/dev/null 2>&1 \
      || log "WARNING: export of $name failed"
  done < <(lxc list --format csv -c n)
fi

# ── manifest + checksums ─────────────────────────────────────────────────────
(
  cd "$TMP"
  find . -type f ! -name SHA256SUMS ! -name manifest.json -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS
  size=$(du -sb . | cut -f1)
  commit=$(git -C /opt/remote.futrx rev-parse HEAD 2>/dev/null || echo unknown)
  printf '{"snapshot":"%s","startedAt":"%s","finishedAt":"%s","host":"%s","dataDir":"%s","projectsDir":"%s","withContainers":%s,"backendQuiesced":%s,"compression":"%s","bytes":%s,"installedCommit":"%s"}\n' \
    "$STAMP" "$started_at" "$(date -u +%FT%TZ)" "$(hostname -f 2>/dev/null || hostname)" \
    "$DATA_DIR" "$PROJECTS_DIR" "$WITH_CONTAINERS" "$STOP_BACKEND" "$EXT" "$size" "$commit" > manifest.json
)
mv "$TMP" "$DEST"
log "snapshot $STAMP complete ($(du -sh "$DEST" | cut -f1))"

# ── retention: newest KEEP_DAILY + one per ISO week for KEEP_WEEKLY weeks ────
mapfile -t snaps < <(find "$BACKUP_ROOT" -mindepth 1 -maxdepth 1 -type d -name '[0-9]*T[0-9]*Z' | sort -r)
declare -A keep_week=()
idx=0
for s in "${snaps[@]}"; do
  base=$(basename "$s"); keep=0
  [ "$idx" -lt "$KEEP_DAILY" ] && keep=1
  wk=$(date -u -d "${base:0:8}" +%G-%V 2>/dev/null || echo "$base")
  if [ -z "${keep_week[$wk]:-}" ] && [ "${#keep_week[@]}" -lt "$KEEP_WEEKLY" ]; then keep_week[$wk]=1; keep=1; fi
  if [ "$keep" = 0 ]; then rm -rf "$s"; log "pruned $base"; fi
  idx=$((idx+1))
done

# ── optional offsite copy ────────────────────────────────────────────────────
if [ -n "$RCLONE_TARGET" ]; then
  if command -v rclone >/dev/null 2>&1; then
    if rclone copy "$DEST" "$RCLONE_TARGET/$STAMP" --transfers 2 --checkers 4 -q; then
      log "offsite copy to $RCLONE_TARGET/$STAMP done"
    else
      log "WARNING: offsite copy failed"
    fi
  else
    log "WARNING: RCLONE_TARGET set but rclone not installed"
  fi
fi

echo "$DEST"
