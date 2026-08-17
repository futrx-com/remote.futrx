# Stack: Laravel

This workspace was provisioned from the `laravel` project template.

## What is installed

| Piece | Detail |
| --- | --- |
| PHP | 8.3 CLI (`php`) with `mysql`, `sqlite3`, `curl`, `mbstring`, `xml`, `zip`, `intl`, `bcmath`, `gd` |
| Composer | `/usr/local/bin/composer` |
| Database | MariaDB, local only, started by systemd (`systemctl status mariadb`) |
| DB auth | root, **no password**; database `laravel` already created |
| Application | `/workspace/app` (`laravel/laravel` skeleton, `.env` already pointed at the local database) |
| Node | 22 with `npm`, from the base image — used by Vite |

The scaffold step is skipped when `/workspace` already has content, so a
project cloned from git keeps its own application.

## Rules that matter here

- Keep everything under `/workspace`. The rest of the container filesystem is
  replaced on workspace upgrade; this provisioning then re-runs automatically.
- `.env` uses `DB_HOST=127.0.0.1` with user `root` and an empty password. The
  database is reachable only from inside this container.
- Run Composer with `COMPOSER_ALLOW_SUPERUSER=1` — the container is root-only.

## Common tasks

```bash
cd /workspace/app

php artisan migrate
php artisan key:generate

# Dev server. Bind 0.0.0.0 or the preview proxy cannot reach it.
php artisan serve --host 0.0.0.0 --port 8000

# Vite assets
npm install && npm run dev -- --host 0.0.0.0
```

## Previews

Ports 8000 (`artisan serve`) and 5173 (Vite) are this template's conventions;
any listening port on `0.0.0.0` becomes a preview URL. Run long-lived servers
as transient systemd units so they outlive the agent turn that started them:

```bash
systemd-run --unit=laravel-serve --working-directory=/workspace/app \
  php artisan serve --host 0.0.0.0 --port 8000
```
