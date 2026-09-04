#!/usr/bin/env bash
# remote-restore — restore a snapshot made by remote-backup onto this host.
#
# Usage: remote-restore <snapshot-dir> [--data] [--projects] [--hostcfg] [--containers] [--yes]
#   With no part flags, restores data + projects + hostcfg (not containers).
#
# Steps: verify checksums → stop backend → extract selected parts in place
# (existing DATA_DIR / PROJECTS_DIR are moved aside to *.pre-restore-<ts>) →
# fix ownership of workspace dirs to the unprivileged idmap (uid 1000000) →
# start backend. Project containers are re-created by the app from the base
# image on demand, or imported from the snapshot with --containers.
set -euo pipefail

SNAP="${1:-}"
[ -n "$SNAP" ] && [ -d "$SNAP" ] || { echo "usage: remote-restore <snapshot-dir> [--data|--projects|--hostcfg|--containers] [--yes]" >&2; exit 2; }
shift
[ "$(id -u)" = 0 ] || { echo "must run as root" >&2; exit 1; }

CONF=/etc/remote-backup.env
[ -f "$CONF" ] && . "$CONF"
DATA_DIR="${DATA_DIR:-/opt/remote.futrx/data}"
PROJECTS_DIR="${PROJECTS_DIR:-/var/lib/remote/projects}"
SERVICE="${SERVICE:-remote.futrx}"

DO_DATA=0; DO_PROJ=0; DO_HOST=0; DO_CONT=0; YES=0
for a in "$@"; do
  case "$a" in
    --data) DO_DATA=1 ;; --projects) DO_PROJ=1 ;; --hostcfg) DO_HOST=1 ;; --containers) DO_CONT=1 ;; --yes) YES=1 ;;
    *) echo "unknown arg $a" >&2; exit 2 ;;
  esac
done
if [ "$DO_DATA$DO_PROJ$DO_HOST$DO_CONT" = "0000" ]; then DO_DATA=1; DO_PROJ=1; DO_HOST=1; fi

cd "$SNAP"
echo "==> verifying checksums"
sha256sum -c --quiet SHA256SUMS
echo "==> manifest:"; cat manifest.json; echo
if [ "$YES" != 1 ]; then
  read -rp "Restore into this host (data=$DO_DATA projects=$DO_PROJ hostcfg=$DO_HOST containers=$DO_CONT)? [y/N] " r
  [ "${r,,}" = "y" ] || exit 1
fi

TS=$(date -u +%Y%m%dT%H%M%SZ)
part() { ls "$SNAP/$1".tar.* 2>/dev/null | head -1; }
untar() { case "$1" in *.zst) tar --zstd -xf "$1" -C "$2" ;; *) tar -xzf "$1" -C "$2" ;; esac; }

systemctl stop "$SERVICE" 2>/dev/null || true

if [ "$DO_DATA" = 1 ]; then
  f=$(part data)
  if [ -n "$f" ]; then
    [ -d "$DATA_DIR" ] && mv "$DATA_DIR" "$DATA_DIR.pre-restore-$TS"
    mkdir -p "$(dirname "$DATA_DIR")"
    untar "$f" "$(dirname "$DATA_DIR")"
    chmod 700 "$DATA_DIR"
    echo "  data restored (previous kept at $DATA_DIR.pre-restore-$TS)"
  fi
fi

if [ "$DO_PROJ" = 1 ]; then
  f=$(part projects)
  if [ -n "$f" ]; then
    [ -d "$PROJECTS_DIR" ] && mv "$PROJECTS_DIR" "$PROJECTS_DIR.pre-restore-$TS"
    mkdir -p "$(dirname "$PROJECTS_DIR")"
    untar "$f" "$(dirname "$PROJECTS_DIR")"
    # workspace + agent-home trees must be owned by the container idmap root
    find "$PROJECTS_DIR" -mindepth 2 -maxdepth 2 \( -name workspace -o -name agent-home \) \
      -exec chown -R 1000000:1000000 {} + 2>/dev/null || true
    echo "  projects restored (previous kept at $PROJECTS_DIR.pre-restore-$TS)"
  fi
fi

if [ "$DO_HOST" = 1 ]; then
  f=$(part hostcfg)
  if [ -n "$f" ]; then
    T=$(mktemp -d)
    untar "$f" "$T"
    if [ -d "$T/hostcfg/root" ]; then cp -a "$T/hostcfg/root/." /root/; fi
    [ -f "$T/hostcfg/etc/remote-backup.env" ] && cp -a "$T/hostcfg/etc/remote-backup.env" /etc/
    echo "  provider tokens + backup env restored"
    echo "  (Caddyfile / systemd units are re-rendered by install.sh; reference copies in $T/hostcfg/etc)"
  fi
fi

if [ "$DO_CONT" = 1 ] && [ -d containers ]; then
  for c in containers/*.tar.gz; do
    [ -f "$c" ] || continue
    n=$(basename "$c" .tar.gz)
    echo "  importing container $n"
    lxc delete -f "$n" 2>/dev/null || true
    lxc import "$c" "$n" || echo "  WARNING: import $n failed"
  done
fi

systemctl start "$SERVICE"
sleep 3
systemctl is-active --quiet "$SERVICE" && echo "==> restore complete; backend running" || { echo "==> backend failed to start — check: journalctl -u $SERVICE -n 50" >&2; exit 1; }
