#!/usr/bin/env bash
# Idempotent PostgreSQL provisioner. Runs as root inside the target container.
#
# Contract (see applications.InstallSpec / installer.go):
#   - APP_INTERNAL_PORT   port PostgreSQL must listen on inside the container
#   - POSTGRES_PASSWORD   superuser password (required)
#   - POSTGRES_USER       superuser name        (default: postgres)
#   - POSTGRES_DB         database to ensure     (default: app)
#
# The script is re-run on every "start"/reconcile, so every step is idempotent.
set -euo pipefail

APP_INTERNAL_PORT="${APP_INTERNAL_PORT:-5432}"
POSTGRES_USER="${POSTGRES_USER:-postgres}"
POSTGRES_DB="${POSTGRES_DB:-app}"

if [ -z "${POSTGRES_PASSWORD:-}" ]; then
  echo "install: POSTGRES_PASSWORD is required" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive

# --- install the server if it is not present -------------------------------
if ! command -v pg_ctlcluster >/dev/null 2>&1; then
  apt-get update -qq
  apt-get install -y --no-install-recommends postgresql postgresql-client
fi

# Resolve the installed cluster (version/name), creating a default one if none.
PGVER="$(ls /etc/postgresql 2>/dev/null | sort -V | tail -n1 || true)"
if [ -z "${PGVER}" ]; then
  PGVER="$(ls /usr/lib/postgresql 2>/dev/null | sort -V | tail -n1)"
  pg_createcluster "${PGVER}" main
fi
CLUSTER="$(ls /etc/postgresql/${PGVER} 2>/dev/null | head -n1 || echo main)"
CONF_DIR="/etc/postgresql/${PGVER}/${CLUSTER}"

# --- bind to the requested internal port and accept LAN connections --------
conf_set() { # key value
  local key="$1" val="$2" f="${CONF_DIR}/postgresql.conf"
  if grep -Eq "^\s*#?\s*${key}\s*=" "$f"; then
    sed -ri "s|^\s*#?\s*(${key})\s*=.*|\1 = ${val}|" "$f"
  else
    printf '%s = %s\n' "$key" "$val" >>"$f"
  fi
}
conf_set port "${APP_INTERNAL_PORT}"
conf_set listen_addresses "'*'"

# Allow password auth from the container/bridge network (the proxy device
# connects from inside the container, so 127.0.0.1 covers the proxy path).
HBA="${CONF_DIR}/pg_hba.conf"
if ! grep -q "futrx-app" "$HBA"; then
  {
    echo "# futrx-app: allow password auth over TCP"
    echo "host all all 127.0.0.1/32 scram-sha-256"
    echo "host all all ::1/128      scram-sha-256"
    echo "host all all 0.0.0.0/0    scram-sha-256"
  } >>"$HBA"
fi

# --- start the cluster ------------------------------------------------------
systemctl enable postgresql >/dev/null 2>&1 || true
pg_ctlcluster "${PGVER}" "${CLUSTER}" restart || systemctl restart postgresql

# Wait until it is accepting connections before configuring roles.
for _ in $(seq 1 30); do
  if pg_isready -h 127.0.0.1 -p "${APP_INTERNAL_PORT}" >/dev/null 2>&1; then break; fi
  sleep 1
done

# --- ensure superuser password + database (idempotent) ----------------------
psql_super() { sudo -u postgres psql -p "${APP_INTERNAL_PORT}" -v ON_ERROR_STOP=1 "$@"; }

if [ "${POSTGRES_USER}" = "postgres" ]; then
  psql_super -c "ALTER USER postgres WITH PASSWORD '${POSTGRES_PASSWORD//\'/\'\'}';"
else
  psql_super -c "DO \$\$ BEGIN
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = '${POSTGRES_USER}') THEN
      ALTER ROLE \"${POSTGRES_USER}\" WITH LOGIN SUPERUSER PASSWORD '${POSTGRES_PASSWORD//\'/\'\'}';
    ELSE
      CREATE ROLE \"${POSTGRES_USER}\" WITH LOGIN SUPERUSER PASSWORD '${POSTGRES_PASSWORD//\'/\'\'}';
    END IF;
  END \$\$;"
fi

if ! psql_super -tAc "SELECT 1 FROM pg_database WHERE datname = '${POSTGRES_DB}'" | grep -q 1; then
  psql_super -c "CREATE DATABASE \"${POSTGRES_DB}\" OWNER \"${POSTGRES_USER}\";"
fi

echo "install: postgresql ready on port ${APP_INTERNAL_PORT} (db=${POSTGRES_DB}, user=${POSTGRES_USER})"
