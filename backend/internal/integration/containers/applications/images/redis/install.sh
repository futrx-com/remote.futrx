#!/usr/bin/env bash
# Idempotent Redis provisioner. Runs as root inside the target container.
#
# Contract:
#   - APP_INTERNAL_PORT   port Redis must listen on inside the container
#   - REDIS_PASSWORD      optional AUTH password (empty = no auth)
set -euo pipefail

APP_INTERNAL_PORT="${APP_INTERNAL_PORT:-6379}"

export DEBIAN_FRONTEND=noninteractive

if ! command -v redis-server >/dev/null 2>&1; then
  apt-get update -qq
  apt-get install -y --no-install-recommends redis-server
fi

DROPIN="/etc/redis/redis.conf.d"
mkdir -p "$DROPIN"
CONF="$DROPIN/zz-futrx-app.conf"
{
  echo "port ${APP_INTERNAL_PORT}"
  echo "bind 0.0.0.0 -::1"
  echo "protected-mode no"
  if [ -n "${REDIS_PASSWORD:-}" ]; then
    echo "requirepass ${REDIS_PASSWORD}"
  fi
} >"$CONF"

# Debian's unit reads /etc/redis/redis.conf; make sure it includes the drop-in.
# The include must be the last directive so it overrides earlier defaults, and
# it must be on its own line with no trailing comment: Redis parses every
# whitespace-separated token as an argument and rejects `include` with extras.
# Strip any prior (possibly malformed) include line first so re-runs self-heal.
sed -i '\#^include .*redis\.conf\.d/zz-futrx-app\.conf#d' /etc/redis/redis.conf 2>/dev/null || true
printf '\ninclude %s\n' "${CONF}" >>/etc/redis/redis.conf

systemctl enable redis-server >/dev/null 2>&1 || true
systemctl restart redis-server

for _ in $(seq 1 30); do
  if redis-cli -p "${APP_INTERNAL_PORT}" ${REDIS_PASSWORD:+-a "${REDIS_PASSWORD}"} ping 2>/dev/null | grep -q PONG; then
    break
  fi
  sleep 1
done

echo "install: redis ready on port ${APP_INTERNAL_PORT}"
