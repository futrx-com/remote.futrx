# 04 — Install scripts

Applications may provide one for custom container provisioning. A Go program in
`backend/container/` needs no shell: Remote packages and builds it automatically.
UI-only and host-backend-only applications need neither — see
[03 — Application capabilities](03-application-capabilities.md).

Hello Remote combines both paths. Its Go programs are built from
`backend/container/`, its complete systemd service lives in `application.json`,
and `infra/install.sh` creates the application-specific system account and
persistent state directory the service uses. The script also records its
provisioned version there so the example UI can prove that it ran. A custom
script is for work the manifest cannot express, such as OS packages, users,
mounts, data migrations, or application-specific configuration. Do not add
shell merely to build a container program, create a unit, manage workspace-idle
declarations, or run a health check; Remote owns those shared operations.

## The contract

The script is piped into `bash -s` **as root inside the target container**:

```
lxc exec <container> --env APP_INTERNAL_PORT=3306 --env … -- bash -s
```

It receives:

- `APP_APPLICATION_ID`, `APP_APPLICATION_NAME`, and
  `APP_APPLICATION_VERSION` — package identity from `application.json`.
- `APP_SERVICE` — the manifest's systemd unit name, or empty when it declares
  none. Use this instead of repeating the unit name in the script.
- `APP_INTERNAL_PORT` — the port the app must bind **inside** the container.
  Always `port.internal` from `application.json`. When no port is declared it
  is `0` and means nothing.
- One variable per `env[]` entry, already resolved: defaults applied, secrets
  generated, required values checked.

It must:

1. **Be idempotent.** It re-runs on an install, retry, or versioned upgrade. A
   second run must converge, not duplicate or reset state. An ordinary start
   does not re-run a current script; it starts the declared service directly.
2. **Bind `APP_INTERNAL_PORT` on all interfaces** (`0.0.0.0`), so the LXD proxy
   device can forward the host port to it. Binding only to `127.0.0.1` inside
   the container makes the app unreachable from the host. Portless
   infrastructure skips this entirely: nothing listens, and there is no proxy device.
3. **Exit non-zero on failure.** A non-zero exit marks the instance `error` and
   surfaces the tail of the output in the UI. This matters more without a port,
   which has no `healthcheck` to fall back on: the script is the only thing
   that can decide the install worked. A mount tool waits for its mountpoint to
   appear and fails if it never does.

Standard output and standard error are captured; the last 2000 characters are
attached to the error message when the script fails.

There is an 8-minute timeout (`execTimeout` in `installer.go`), which is
generous enough for an `apt-get install` on a cold container.

## Container-side Go programs

The capability path `backend/container/` is enforced: Remote discovers
container Go source only there. Inside it, `cmd/` is optional. Remote supports
two executable layouts:

| Programs | Source layout | Installed binary |
|---|---|---|
| One | Root package in `backend/container/` | `/usr/local/bin/<application-id>` |
| One or more explicitly named programs | `backend/container/cmd/<binary>/` | `/usr/local/bin/<binary>` for every immediate `cmd/` child |

Each executable package must use `package main` and provide `func main()`. A
single-program application can therefore be as small as:

```text
backend/
  container/
    main.go
```

Use the standard Go `cmd/` layout when the binary name should differ from the
application ID or when the application installs multiple programs:

```text
backend/
  container/
    cmd/
      my-agent/
        main.go
    internal/
      state/
        state.go
```

This choice is filesystem-driven; there is no manifest field for it. Other
subdirectories may hold imported Go packages, but Remote does not install them
as independent executables.

Remote deterministically packs the source, stages it in the target LXD
container, installs the matching Go toolchain, and builds either the root
package or every immediate `cmd/*` package. If any `cmd/*` program exists, the
root package is not built as an executable. It may still contain importable
library code, though `internal/` is the conventional home for implementation
shared by the commands. Do not put a second `package main` at the root and
expect Remote to install it alongside the `cmd/*` binaries.

Remote places the resulting binaries in `/usr/local/bin`. It records a
source-derived build marker, so idempotence does not require `--version`, a
version variable, `package.sh`, or a committed archive. An optional
`infra/install.sh` runs after these generated build steps when the application
also needs custom provisioning. Remote materializes the manifest's `service`
only after both have completed, so its command may safely reference a newly
built or installed binary.

Applications uploaded as ZIPs may ship their own `backend/container/go.mod` for
dependencies. The built-in catalog's container source participates in the
catalog module and receives a generated module when staged.

## Legacy bundled infra files

An application may include `infra/payload.tar.gz`. The archive holds
regular files and directories under `infra/`. The catalog validates the
archive and wraps the install script to extract it into a temporary directory
inside the target container. The script receives that directory as
`APP_PACKAGE_DIR`; cleanup runs when the script exits, including on failure.
Applications without an archive retain the plain `bash -s` behavior.

This transport remains supported for existing uploaded packages. New Go-based
container programs should use `backend/container/`; it removes the possibility
of committing a payload that is stale relative to its source.

Payloads are limited to 8 MiB compressed and 32 MiB expanded. Paths outside
`infra/`, links, duplicate entries and special files are rejected. An
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

echo "install: myapp provisioned"
```

## Idempotency in practice

These are the patterns that make a re-run safe:

| Do | Instead of |
|---|---|
| `if ! command -v x; then apt-get install x; fi` | `apt-get install x` unconditionally (slow, but safe) |
| `cat > /etc/app/zz-futrx.conf` (a file you own) | `>> /etc/app/app.conf` (appends grow every run) |
| `CREATE DATABASE IF NOT EXISTS` | `CREATE DATABASE` |
| `CREATE USER IF NOT EXISTS` / `ALTER USER` | `CREATE USER` |
| `useradd -r app 2>/dev/null || true` | `useradd -r app` |

The MySQL application is a worked example of the harder case: on a fresh install root
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

## Infrastructure with no port to wait for

A portless infrastructure script uses the same contract minus the port. A mount
application may install `fuse3`, place its binary, and wait for `mountpoint -q`
before exiting. A long-running process belongs in the manifest's `service`
object. Remote then creates its root-owned `0600` environment file, unit, and
workspace-idle declaration consistently; the custom script does none of that.

## Healthcheck

`healthcheck.command` in `application.json` is a readiness probe run inside the
container. `{{internalPort}}` is substituted:

```json
"healthcheck": { "command": "mysqladmin ping -h 127.0.0.1 -P {{internalPort}} --silent" }
```

It runs after the install script and manifest service have been installed on an
install, and after the service is started on a start, with the same resolved
environment. It is retried
every two seconds for up to a minute; an app whose probe never passes is
reported as failed rather than as running.

It is the platform-owned readiness gate. A custom install script should wait
only for work that it owns itself, such as a mount or migration; it must not
start or poll the manifest-owned service.

## Testing a script

The install script only runs against a real container, so it needs a host with
a working LXD. Catalog tests validate the declarative service and generated
unit behavior without LXD; they do **not** execute a custom script.

The fastest loop:

1. Install the application at project scope from the UI, against a project whose
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
- **Data destruction on re-run.** Installs, retries, and upgrades may all run it again.
- **Network assumptions beyond the container.** A dedicated global container
  has network by the time the script runs (the installer waits for an IPv4
  route), but nothing else is guaranteed.
