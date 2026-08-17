# Ubuntu 24.04 ships PHP 8.3 in its own repositories, so no third-party PPA
# is needed. Keep the extension list to what WordPress and WooCommerce
# actually require; each one costs disk on a small host.
echo "--- installing PHP 8.3 and MariaDB ---"
apt_retry update -qq
apt_retry install -y -qq \
    php8.3-cli php8.3-mysql php8.3-curl php8.3-gd php8.3-mbstring \
    php8.3-xml php8.3-zip php8.3-intl php8.3-bcmath php8.3-soap \
    mariadb-server mariadb-client \
    unzip less

# MariaDB must come back on its own after a container reboot. The Debian
# package ships the unit; enabling it is what makes it a boot service.
echo "--- enabling MariaDB ---"
systemctl enable mariadb
systemctl start mariadb || service mariadb start

# Wait for the socket rather than sleeping a fixed amount: a cold first start
# on a 1 vCPU host can take a while.
for _ in $(seq 1 60); do
    if mariadb-admin ping --silent 2>/dev/null || mysqladmin ping --silent 2>/dev/null; then
        break
    fi
    sleep 2
done

# Container-local database. Root authenticates over the unix socket, so there
# is no password to manage; nothing outside the container can reach it.
echo "--- creating the wordpress database ---"
mysql -u root -e "CREATE DATABASE IF NOT EXISTS wordpress DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

echo "--- installing Composer ---"
curl -fsSL https://getcomposer.org/installer -o /tmp/composer-setup.php
php /tmp/composer-setup.php --quiet --install-dir=/usr/local/bin --filename=composer
rm -f /tmp/composer-setup.php

echo "--- installing WP-CLI ---"
curl -fsSL https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar \
    -o /usr/local/bin/wp
chmod 0755 /usr/local/bin/wp

# WP-CLI refuses to run as root without this flag. The container IS root-only,
# so make it the default instead of teaching every caller to pass it.
cat > /usr/local/bin/wp-root <<'WRAPPER'
#!/bin/sh
exec /usr/local/bin/wp --allow-root "$@"
WRAPPER
chmod 0755 /usr/local/bin/wp-root

# WordPress core lands in the durable workspace, so it survives container
# replacement. Never clobber an existing installation.
mkdir -p /workspace/public
if [ ! -f /workspace/public/wp-load.php ]; then
    echo "--- downloading WordPress core ---"
    wp --allow-root --path=/workspace/public core download
else
    echo "WordPress core already present in /workspace/public - skipping download"
fi

if [ ! -f /workspace/public/wp-config.php ]; then
    echo "--- writing wp-config.php ---"
    wp --allow-root --path=/workspace/public config create \
        --dbname=wordpress --dbuser=root --dbhost=localhost --skip-check
fi

# php -S is enough for a single-developer preview and costs far less memory
# than nginx + php-fpm on a 4 GB box. PHP_CLI_SERVER_WORKERS keeps wp-admin
# usable: WordPress makes loopback requests to itself, which deadlock a
# single-worker built-in server.
echo "--- installing the preview web server unit ---"
cat > /etc/systemd/system/remote-wordpress.service <<'UNIT'
[Unit]
Description=WordPress preview server (PHP built-in, port 8080)
After=network.target mariadb.service
Wants=mariadb.service

[Service]
Type=simple
Environment=PHP_CLI_SERVER_WORKERS=4
WorkingDirectory=/workspace/public
ExecStart=/usr/bin/php -S 0.0.0.0:8080 -t /workspace/public
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable remote-wordpress.service
systemctl restart remote-wordpress.service || true

echo "--- versions ---"
php --version | head -1
composer --version
wp --allow-root --version
mysql --version
