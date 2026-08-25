# Application images

This is the catalog of one-click installable apps ("Applications" tab).
It is **embedded into the backend binary** (`//go:embed images` in
`registry.go`), and also surfaced at the repo root as `installable-images/`
(a symlink) so the catalog is discoverable from the project root.

Each subdirectory is one app:

```
images/
  postgresql/
    image.json     # metadata: name, description, port, env, systemd service
    install.sh     # idempotent installer, run as root inside the target container
```

## Adding a new app

1. Create `images/<id>/image.json`. The `id` must equal the directory name.
2. Create the install script referenced by `install` (default `install.sh`).
3. Rebuild the backend. `NewRegistry()` validates every entry at startup, so a
   malformed `image.json` fails the build/tests loudly (see `registry_test.go`).

No other code changes are required — the new app appears in the catalog
automatically for both global and project installs.

## `image.json` schema

| field | type | notes |
|---|---|---|
| `id` | string | must match the directory name |
| `name` | string | display name |
| `description` | string | one-line summary |
| `category` | string | e.g. `database`, `cache` (UI grouping) |
| `version` | string | shown in the UI |
| `icon` | string | icon key used by the frontend |
| `scopes` | string[] | any of `global`, `project` |
| `port.internal` | int | port the software listens on **inside** the container |
| `port.defaultExternal` | int | preferred **host** port; auto-bumped on conflict |
| `port.protocol` | string | `tcp` (default) or `udp` |
| `port.bindAddress` | string | host interface for the proxy (default `127.0.0.1`) |
| `env[]` | object[] | install-time inputs (see below) |
| `service` | string | systemd unit name inside the container (start/stop/status) |
| `connection` | object | maps env vars to canonical connection fields (see below) |
| `install` | string | install-script filename (default `install.sh`) |
| `base` | string | LXD image for a **dedicated global** container (default `ubuntu:24.04`) |
| `healthcheck.command` | string | readiness probe run inside the container |

### `env[]` entries

| field | notes |
|---|---|
| `key` | environment variable passed to the install script |
| `label` | UI label |
| `required` | reject install if left blank and no default/generator |
| `secret` | value is redacted in API responses (passwords) |
| `default` | applied when the user leaves the field blank |
| `generate` | `password` → a strong value is generated when blank |

### `connection` object

Lets the UI show a uniform user/password/database panel for every server,
regardless of how the image names its env vars:

| field | notes |
|---|---|
| `user` | static username when there is no configurable one (e.g. MySQL `root`) |
| `userEnv` | env var holding the username (takes precedence over `user`) |
| `passwordEnv` | env var holding the password |
| `databaseEnv` | env var holding the default database, if any |

## Install-script contract

The script runs as root inside the target container via `bash -s`, with:

- `APP_INTERNAL_PORT` — the port the app must bind **inside** the container
  (always `port.internal`).
- one variable per `env[]` key, already resolved (defaults applied, secrets
  generated).

It must be **idempotent** — it is re-run on every start/reconcile — and it must
make the app listen on `APP_INTERNAL_PORT` on all interfaces so the LXD proxy
device can forward the host port to it.

## How exposure works

Every installed app is reached on the host through an LXD **proxy device**:

```
listen = <protocol>:<bindAddress>:<externalPort>   # on the host
connect = <protocol>:127.0.0.1:<internalPort>      # inside the container
```

- **Global** apps run in their own dedicated `futrx-app-<id>` container.
- **Project** apps are installed inside that project's container (and are also
  reachable on the LXD bridge at `<slug>.lxd:<internalPort>`).

The host `externalPort` is allocated to avoid collisions with other apps and
with anything already listening on the host, starting from `defaultExternal`.
