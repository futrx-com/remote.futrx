# Stack: WordPress

This workspace was provisioned from the `wordpress` project template.

## What is installed

| Piece | Detail |
| --- | --- |
| PHP | 8.3 CLI (`php`) with `mysql`, `curl`, `gd`, `mbstring`, `xml`, `zip`, `intl`, `bcmath`, `soap` |
| Database | MariaDB, local only, started by systemd (`systemctl status mariadb`) |
| DB auth | root over the unix socket, **no password** — `mysql -u root` just works |
| Database name | `wordpress` (already created) |
| WP-CLI | `/usr/local/bin/wp`; use `wp --allow-root` or the `wp-root` wrapper |
| Composer | `/usr/local/bin/composer` |
| Site root | `/workspace/public` (WordPress core downloaded, `wp-config.php` written) |
| Preview server | `remote-wordpress.service`, PHP built-in server on **port 8080** |

## Rules that matter here

- Everything you want to keep must live under `/workspace`. The rest of the
  container filesystem is replaced whenever the workspace is upgraded — this
  provisioning re-runs automatically after that happens.
- Run WP-CLI as `wp --allow-root ...` from `/workspace/public`, or use
  `wp-root`, which adds the flag for you.
- The database connection is `--dbhost=localhost` so PHP uses the unix socket.
  Do not switch it to `127.0.0.1`.
- The preview server is a systemd unit. After changing PHP files nothing needs
  restarting; after changing the unit, run
  `systemctl restart remote-wordpress`.
- Do not start a second server on 8080 — check with `ss -ltnp` first.

## Common tasks

```bash
cd /workspace/public

# Finish the install non-interactively
wp --allow-root core install \
  --url=http://localhost:8080 --title="Dev site" \
  --admin_user=admin --admin_password=admin --admin_email=dev@example.com

# WooCommerce is not preinstalled; add it when the project needs a store
wp --allow-root plugin install woocommerce --activate

# Database access
mysql -u root wordpress
wp --allow-root db query "SELECT option_value FROM wp_options WHERE option_name='siteurl'"
```

## Previews

Port 8080 is published as a preview URL by the platform. Bind any additional
server to `0.0.0.0`, never `127.0.0.1`, or it will not be reachable.
