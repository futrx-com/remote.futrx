# 04 — Install scripts

Only `type: "service"` images have one. A `ui` image installs nothing and needs
no script — see [03 — Image types](03-image-types.md).

## The contract

The script is piped into `bash -s` **as root inside the target container**:

```
lxc exec <container> --env APP_INTERNAL_PORT=3306 --env … -- bash -s
```

It receives:

- `APP_INTERNAL_PORT` — the port the app must bind **inside** the container.
  Always `port.internal` from `image.json`.
- One variable per `env[]` entry, already resolved: defaults applied, secrets
  generated, required values checked.

It must:

1. **Be idempotent.** It re-runs on every install *and every start*. A second
   run must be a no-op, not a reinstall or a reset.
2. **Bind `APP_INTERNAL_PORT` on all interfaces** (`0.0.0.0`), so the LXD proxy
   device can forward the host port to it. Binding only to `127.0.0.1` inside
   the container makes the app unreachable from the host.
3. **Exit non-zero on failure.** A non-zero exit marks the instance `error` and
   surfaces the tail of the output in the UI.

Standard output and standard error are captured; the last 2000 characters are
attached to the error message when the script fails.

There is an 8-minute timeout (`execTimeout` in `installer.go`), which is
generous enough for an `apt-get install` on a cold container.

## Skeleton

```bash
#!/usr/bin/env bash
# Idempotent <app> provisioner. Runs as root inside the target container.
#
# Contract:
#   - APP_INTERNAL_PORT   port to listen on inside the container
#   - MY_APP_PASSWORD     required, generated when blank
set -euo pipefail

APP_INTERNAL_PORT="${APP_INTERNAL_PORT:-5432}"

if [ -z "${MY_APP_PASSWORD:-}" ]; then
  echo "install: MY_APP_PASSWORD is required" >&2
  exit 1
fi

export DEBIAN_FRONTEND=noninteractive

# 1. Install the software, but only if it is missing.
if ! command -v myapp >/dev/null 2>&1; then
  apt-get update -qq
  apt-get install -y --no-install-recommends myapp
fi

# 2. Write configuration. Overwriting a file you own is idempotent;
#    appending to one is not.
cat >/etc/myapp/zz-futrx.conf <<EOF
port = ${APP_INTERNAL_PORT}
listen = 0.0.0.0
EOF

# 3. Start it under systemd.
systemctl enable myapp >/dev/null 2>&1 || true
systemctl restart myapp

# 4. Wait until it is actually accepting connections.
for _ in $(seq 1 30); do
  if (exec 3<>"/dev/tcp/127.0.0.1/${APP_INTERNAL_PORT}") 2>/dev/null; then
    exec 3<&-
    break
  fi
  sleep 1
done

echo "install: myapp ready on port ${APP_INTERNAL_PORT}"
```

## Idempotency in practice

These are the patterns that make a re-run safe:

| Do | Instead of |
|---|---|
| `if ! command -v x; then apt-get install x; fi` | `apt-get install x` unconditionally (slow, but safe) |
| `cat > /etc/app/zz-futrx.conf` (a file you own) | `>> /etc/app/app.conf` (appends grow every run) |
| `CREATE DATABASE IF NOT EXISTS` | `CREATE DATABASE` |
| `CREATE USER IF NOT EXISTS` / `ALTER USER` | `CREATE USER` |
| `systemctl restart` | `systemctl start` (a no-op if config changed) |
| `useradd -r app 2>/dev/null || true` | `useradd -r app` |

The MySQL image is a worked example of the harder case: on a fresh install root
authenticates over a unix socket, but once a password is set it authenticates
with that password. Its `run_sql` helper tries both, so re-runs succeed either
way — see [`mysql/install.sh`](../mysql/install.sh).

## Secrets

Values marked `secret: true` in `env[]` arrive as ordinary environment
variables. Two rules:

- **Never echo them.** The script's output is stored on the instance and shown
  in the UI on failure.
- **Escape them** before interpolating into SQL or configuration. MySQL's
  script does `ESC_PW="${MYSQL_ROOT_PASSWORD//\'/\'\'}"` before putting the
  password in a statement.

The same applies to non-secret user input. `ui-playground` used to escape
`PLAYGROUND_TITLE` before writing it into HTML for exactly this reason.

## Healthcheck

`healthcheck.command` in `image.json` is a readiness probe run inside the
container. `{{internalPort}}` is substituted:

```json
"healthcheck": { "command": "mysqladmin ping -h 127.0.0.1 -P {{internalPort}} --silent" }
```

It is a separate, cheap check — the install script should still wait for its
own service to come up before exiting, as in the skeleton above.

## Testing a script

The install script only runs against a real container, so it needs a host with
a working LXD. The catalog tests do **not** execute it; they only assert it
exists and is readable.

The fastest loop:

1. Install the image at project scope from the UI, against a project whose
   container is running.
2. On failure, the error and the tail of the script output appear on the
   installed row.
3. `lxc exec <container> -- bash` to inspect state, then hit **Start** to re-run
   the script — which also proves idempotency.

## What not to put in an install script

- **Anything host-side.** The script runs inside a container and cannot see the
  host.
- **Interactive prompts.** There is no TTY. Use `DEBIAN_FRONTEND=noninteractive`.
- **Data destruction on re-run.** Remember it runs on every start.
- **Network assumptions beyond the container.** A dedicated global container
  has network by the time the script runs (the installer waits for an IPv4
  route), but nothing else is guaranteed.
