#!/usr/bin/env bash
# Idempotent MySQL provisioner. Runs as root inside the target container.
#
# Contract:
#   - APP_INTERNAL_PORT    port MySQL must listen on inside the container
#   - MYSQL_ROOT_PASSWORD  root password (required)
#   - MYSQL_DATABASE       database to ensure (default: app)
set -euo pipefail

APP_INTERNAL_PORT="${APP_INTERNAL_PORT:-3306}"
MYSQL_DATABASE="${MYSQL_DATABASE:-app}"

if [ -z "${MYSQL_ROOT_PASSWORD:-}" ]; then
  echo "install: MYSQL_ROOT_PASSWORD is required" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive

if ! command -v mysqld >/dev/null 2>&1; then
  apt-get update -qq
  apt-get install -y --no-install-recommends mysql-server mysql-client
fi

# --- bind to the requested internal port, listen on all interfaces ----------
CONF="/etc/mysql/mysql.conf.d/zz-futrx-app.cnf"
cat >"$CONF" <<EOF
[mysqld]
port = ${APP_INTERNAL_PORT}
bind-address = 0.0.0.0
mysqlx = OFF
EOF

systemctl enable mysql >/dev/null 2>&1 || true
systemctl restart mysql

for _ in $(seq 1 30); do
  if mysqladmin ping -h 127.0.0.1 -P "${APP_INTERNAL_PORT}" --silent >/dev/null 2>&1; then break; fi
  sleep 1
done

# --- set root password + ensure database (idempotent) -----------------------
# Fresh installs authenticate root via unix_socket; once a password is set we
# authenticate with it. Try both so re-runs succeed.
run_sql() {
  if mysql --protocol=socket -uroot -e "SELECT 1" >/dev/null 2>&1; then
    mysql --protocol=socket -uroot "$@"
  else
    mysql -h 127.0.0.1 -P "${APP_INTERNAL_PORT}" -uroot -p"${MYSQL_ROOT_PASSWORD}" "$@"
  fi
}

ESC_PW="${MYSQL_ROOT_PASSWORD//\'/\'\'}"
run_sql -e "ALTER USER 'root'@'localhost' IDENTIFIED WITH caching_sha2_password BY '${ESC_PW}';"
run_sql -e "CREATE USER IF NOT EXISTS 'root'@'%' IDENTIFIED WITH caching_sha2_password BY '${ESC_PW}';"
run_sql -e "ALTER USER 'root'@'%' IDENTIFIED WITH caching_sha2_password BY '${ESC_PW}';"
run_sql -e "GRANT ALL PRIVILEGES ON *.* TO 'root'@'%' WITH GRANT OPTION;"
run_sql -e "CREATE DATABASE IF NOT EXISTS \`${MYSQL_DATABASE}\`;"
run_sql -e "FLUSH PRIVILEGES;"

echo "install: mysql ready on port ${APP_INTERNAL_PORT} (db=${MYSQL_DATABASE})"
