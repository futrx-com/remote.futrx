# 04 — Install scripts

`service` and `tool` images have one. A `ui` or `backend` image installs
nothing in a container and needs no script — see
[03 — Image types](03-image-types.md).

## The contract

The script is piped into `bash -s` **as root inside the target container**:

```
lxc exec <container> --env APP_INTERNAL_PORT=3306 --env … -- bash -s
```

It receives:

- `APP_INTERNAL_PORT` — the port the app must bind **inside** the container.
  Always `port.internal` from `image.json`. A `tool` has no port, so it is `0`
  and means nothing.
- One variable per `env[]` entry, already resolved: defaults applied, secrets
  generated, required values checked.

It must:

1. **Be idempotent.** It re-runs on every install *and every start*. A second
   run must be a no-op, not a reinstall or a reset.
2. **Bind `APP_INTERNAL_PORT` on all interfaces** (`0.0.0.0`), so the LXD proxy
   device can forward the host port to it. Binding only to `127.0.0.1` inside
   the container makes the app unreachable from the host. *A `tool` skips this
   entirely: nothing listens, and there is no proxy device.*
3. **Exit non-zero on failure.** A non-zero exit marks the instance `error` and
   surfaces the tail of the output in the UI. This matters more for a tool,
   which has no `healthcheck` to fall back on: the script is the only thing
   that can decide the install worked. A mount tool waits for its mountpoint to
   appear and fails if it never does.

Standard output and standard error are captured; the last 2000 characters are
attached to the error message when the script fails.

There is an 8-minute timeout (`execTimeout` in `installer.go`), which is
generous enough for an `apt-get install` on a cold container.

## Bundled container files

An image may include `container.tar.gz` beside `image.json`. The archive holds
regular files and directories under `container/`. The catalog validates the
archive and wraps the install script to extract it into a temporary directory
inside the target container. The script receives that directory as
`APP_PACKAGE_DIR`; cleanup runs when the script exits, including on failure.
Images without an archive retain the plain `bash -s` behavior.

For example, an image whose container-side program is a Go module builds it
from `$APP_PACKAGE_DIR/container/`. That module, the image's host `plugin/` and
its browser `ui/` all belong to the same image folder, and a packaging script
in the image refreshes the archive from that source. The archive is what allows
a catalog to carry nested Go modules, which `go:embed` does not traverse.

The s3disk image is the worked example, and it lives in its own repository
rather than here. Its `container/` is a Go module with its own `go.mod`, its
`package.sh` rebuilds `container.tar.gz` reproducibly, and `install.sh`
compiles the staged source inside the container. Copy that shape if your image
needs one — including the part that is easy to miss: because
`go:embed` skips a nested module in silence rather than failing, a stale or
missing archive produces a green build and a broken install, so the freshness
of the archive needs a test of its own.

Payloads are limited to 8 MiB compressed and 32 MiB expanded. Paths outside
`container/`, links, duplicate entries and special files are rejected. An
uploaded package carries its payload the same way; see
[16 — Uploaded packages](16-uploaded-packages.md).

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
with that password. A `run_sql` helper that tries both is what makes the re-run
succeed either way.

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

## Tools, which have no port to wait for

A `tool` image's script is the same contract minus the port. A mount tool is
the worked example: it installs `fuse3`, puts the binary in place, writes its
credentials to a root-only environment file, generates a systemd unit, and then
**waits for `mountpoint -q` to succeed** before exiting. That wait is the whole
readiness check.

A tool whose daemon runs for the life of the container should also declare
itself to the idle-workspace probe, by writing its process name into
`/etc/remote/workspace-idle.d/<name>`:

```sh
mkdir -p /etc/remote/workspace-idle.d
printf '%s\n' "$UNIT" >"/etc/remote/workspace-idle.d/${UNIT}"
```

The probe treats any unrecognised process as someone working in the project, so
without this an always-running daemon pins every workspace it is installed in
and an idle project is never archived. Remote reads names from that directory
and ships no list of its own — an image that leaves nothing running needs
nothing here.

Note what it does *not* do: it never echoes a secret, and it writes credentials
to a `0600` file rather than into the unit, which is world-readable.

## Healthcheck

`healthcheck.command` in `image.json` is a readiness probe run inside the
container. `{{internalPort}}` is substituted:

```json
"healthcheck": { "command": "mysqladmin ping -h 127.0.0.1 -P {{internalPort}} --silent" }
```

It runs after the install script on an install, and after the service is started
on a start, with the same environment the install script gets. It is retried
every two seconds for up to a minute; an app whose probe never passes is
reported as failed rather than as running.

It is a separate, cheap check — the install script should still wait for its
own service to come up before exiting, as in the skeleton above. The probe is
the margin around that wait, not a replacement for it.

## Testing a script

The install script only runs against a real container, so it needs a host with
a working LXD. The catalog tests do **not** execute it; they only assert it
exists and is readable.

The fastest loop:

1. Install the image at project scope from the UI, against a project whose
   container is running.
2. On failure, the error and the tail of the script output appear on the
   installed row.
3. `lxc exec <container> -- bash` to inspect state, then hit **Retry** on the
   failed row to install again — which also proves idempotency. **Start** does
   not re-run the script.

## What not to put in an install script

- **Anything host-side.** The script runs inside a container and cannot see the
  host.
- **Interactive prompts.** There is no TTY. Use `DEBIAN_FRONTEND=noninteractive`.
- **Data destruction on re-run.** Remember it runs on every start.
- **Network assumptions beyond the container.** A dedicated global container
  has network by the time the script runs (the installer waits for an IPv4
  route), but nothing else is guaranteed.
