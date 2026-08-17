# PHP 8.3 is in the Ubuntu 24.04 repositories; no third-party PPA needed.
# Node 22 (for Vite) is already in the base image.
echo "--- installing PHP 8.3 and MariaDB ---"
apt_retry update -qq
apt_retry install -y -qq \
    php8.3-cli php8.3-mysql php8.3-sqlite3 php8.3-curl php8.3-mbstring \
    php8.3-xml php8.3-zip php8.3-intl php8.3-bcmath php8.3-gd \
    mariadb-server mariadb-client \
    unzip less

echo "--- enabling MariaDB ---"
systemctl enable mariadb
systemctl start mariadb || service mariadb start

for _ in $(seq 1 60); do
    if mariadb-admin ping --silent 2>/dev/null || mysqladmin ping --silent 2>/dev/null; then
        break
    fi
    sleep 2
done

# Container-local database, root over the unix socket, no password.
echo "--- creating the laravel database ---"
mysql -u root -e "CREATE DATABASE IF NOT EXISTS laravel DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

echo "--- installing Composer ---"
curl -fsSL https://getcomposer.org/installer -o /tmp/composer-setup.php
php /tmp/composer-setup.php --quiet --install-dir=/usr/local/bin --filename=composer
rm -f /tmp/composer-setup.php

# Only scaffold into an empty workspace. A project restored from git, or a
# container replaced after the first provisioning, must never be overwritten.
if [ -f /workspace/app/artisan ]; then
    echo "Laravel application already present in /workspace/app - skipping scaffold"
elif [ -n "$(ls -A /workspace 2>/dev/null | grep -v -e '^\.agents$' -e '^\.browser$' -e '^scripts$' -e '^AGENTS\.md$' -e '^CLAUDE\.md$' -e '^README\.md$' || true)" ]; then
    echo "/workspace is not empty - skipping the laravel/laravel scaffold"
else
    echo "--- creating the Laravel application ---"
    COMPOSER_ALLOW_SUPERUSER=1 composer create-project --no-interaction \
        laravel/laravel /workspace/app
fi

if [ -f /workspace/app/.env ]; then
    echo "--- pointing .env at the local MariaDB ---"
    sed -i \
        -e 's/^# *DB_CONNECTION=.*/DB_CONNECTION=mysql/' \
        -e 's/^DB_CONNECTION=.*/DB_CONNECTION=mysql/' \
        -e 's/^# *DB_HOST=.*/DB_HOST=127.0.0.1/' \
        -e 's/^DB_HOST=.*/DB_HOST=127.0.0.1/' \
        -e 's/^# *DB_PORT=.*/DB_PORT=3306/' \
        -e 's/^DB_PORT=.*/DB_PORT=3306/' \
        -e 's/^# *DB_DATABASE=.*/DB_DATABASE=laravel/' \
        -e 's/^DB_DATABASE=.*/DB_DATABASE=laravel/' \
        -e 's/^# *DB_USERNAME=.*/DB_USERNAME=root/' \
        -e 's/^DB_USERNAME=.*/DB_USERNAME=root/' \
        -e 's/^# *DB_PASSWORD=.*/DB_PASSWORD=/' \
        -e 's/^DB_PASSWORD=.*/DB_PASSWORD=/' \
        /workspace/app/.env
fi

echo "--- versions ---"
php --version | head -1
composer --version
node --version
mysql --version
