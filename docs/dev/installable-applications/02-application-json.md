# 02 — `application.json` reference

Every application directory contains exactly one `application.json`. It is loaded and
validated at server startup by `registry.go:loadApplication`; a malformed file fails
the build and the tests rather than producing a broken catalog entry.

The Go type behind it is `Application` in
[`service/applications/model.go`](../../../backend/internal/service/applications/model.go).

## Complete example

An application using every infrastructure and presentation field:

```json
{
  "id": "mysql",
  "name": "MySQL",
  "description": "Popular open-source relational database server.",
  "category": "database",
  "version": "8.0",
  "icon": "database",
  "scopes": ["global", "project"],
  "base": "ubuntu:24.04",
  "port": {
    "internal": 3306,
    "defaultExternal": 3306,
    "protocol": "tcp",
    "bindAddress": "127.0.0.1"
  },
  "env": [
    {
      "key": "MYSQL_ROOT_PASSWORD",
      "label": "Root password",
      "required": true,
      "secret": true,
      "generate": "password"
    },
    {
      "key": "MYSQL_DATABASE",
      "label": "Default database",
      "required": false,
      "default": "app"
    }
  ],
  "service": "mysql",
  "connection": {
    "user": "root",
    "passwordEnv": "MYSQL_ROOT_PASSWORD",
    "databaseEnv": "MYSQL_DATABASE"
  },
  "install": "infra/install.sh",
  "healthcheck": {
    "command": "mysqladmin ping -h 127.0.0.1 -P {{internalPort}} --silent"
  },
  "ui": {
    "entry": "scripts/main.js",
    "styles": ["style/style.css"],
    "views": { "panel": "views/index.html" }
  }
}
```

An application with backend and UI capabilities:

```json
{
  "id": "backend-playground",
  "name": "Backend Playground",
  "description": "Developer fixture: a Go plugin that exercises every part of the backend API.",
  "category": "development",
  "version": "1",
  "icon": "ui/assets/logo.svg",
  "scopes": ["global", "project"],
  "backend": {
    "access": "registered",
    "timeoutMs": 10000
  },
  "ui": {
    "entry": "scripts/main.js",
    "styles": ["style/playground.css"],
    "views": { "panel": "views/panel.html", "console": "views/console.html" }
  }
}
```

An application that provisions into a project's container without exposing a port:

```json
{
  "id": "object-mount",
  "name": "Object Mount",
  "description": "Mount a bucket as a normal filesystem inside this project's container.",
  "category": "storage",
  "version": "0.1.0",
  "icon": "disk",
  "scopes": ["project"],
  "env": [
    { "key": "MOUNT_BUCKET", "label": "Bucket", "required": true },
    { "key": "MOUNT_POINT", "label": "Mount at", "default": "/workspace/bucket" },
    { "key": "AWS_ACCESS_KEY_ID", "label": "Access key ID", "required": true, "secret": true },
    { "key": "AWS_SECRET_ACCESS_KEY", "label": "Secret access key", "required": true, "secret": true }
  ],
  "service": "object-mount",
  "install": "infra/install.sh"
}
```

An application with only a UI capability:

```json
{
  "id": "ui-playground",
  "name": "UI Playground",
  "description": "Developer fixture: contributes to every extension slot.",
  "category": "development",
  "version": "1",
  "icon": "ui/assets/logo.svg",
  "scopes": ["global", "project"],
  "ui": {
    "entry": "scripts/main.js",
    "styles": ["style/playground.css"],
    "views": { "panel": "views/panel.html", "context": "views/context.html" }
  }
}
```

## Top-level fields

| Field | Type | Required | Notes |
|---|---|---|---|
| `id` | string | yes | Must equal the directory name. Loading fails otherwise. An uploaded package must state it explicitly — it has no directory to inherit from. See [16 — Uploaded packages](16-uploaded-packages.md). |
| `name` | string | yes | Display name in the catalog and on installed rows. |
| `description` | string | no | One line; the card truncates to two lines. |
| `category` | string | no | Free text, e.g. `database`, `cache`, `development`. |
| `version` | string | **yes** | A string, not a number — `"8.0"`, `"16"`, `"1.2.3-rc1"`. Shown next to the name, and the signal that re-runs `infra/install.sh` on an installed copy when it changes. See [17 — Versions and upgrades](17-versions-and-upgrades.md). |
| `icon` | string | no | Built-in key or a path into this application's `ui/`. See [09 — Styling and icons](09-styling-and-icons.md). |
| `scopes` | string[] | yes | Any of `global`, `project`. At least one. |
| `base` | string | no | LXD image for a dedicated global infrastructure container. Default `ubuntu:24.04`. |
| `port` | object | no | See below. Requires infrastructure; omit it when nothing is exposed. |
| `env` | object[] | no | Install-time inputs. See below. |
| `service` | string | no | systemd unit name inside the container. Requires infrastructure; it is what stop and uninstall act on. |
| `connection` | object | no | Maps env vars to user/password/database. See below. |
| `install` | string | no | Override for the install-script path inside `infra/`. When omitted, `infra/install.sh` is detected automatically. |
| `healthcheck` | object | no | `{ "command": "…" }` run inside the container. Requires `port.internal`. |
| `ui` | object | no | Overrides what is loaded from `ui/`. See below. |
| `backend` | object | no | Overrides the defaults for the Go plugin in `backend/`. See below. |
| `source` | string | — | **Server-set, not accepted from `application.json`.** `builtin` or `uploaded`; anything declared here is overwritten. |

### `port`

Describes how an application's infrastructure exposes itself. Omit the entire
object when the infrastructure does not listen on a network port.

| Field | Type | Default | Notes |
|---|---|---|---|
| `internal` | int | — | Port the software listens on **inside** the container. Must be > 0. |
| `defaultExternal` | int | `internal` | Preferred **host** port. Bumped automatically on conflict. |
| `protocol` | string | `tcp` | `tcp` or `udp`. |
| `bindAddress` | string | `127.0.0.1` | Host interface the proxy listens on. Keep the default unless the app is meant to be publicly reachable. |

The host port is allocated at install time to avoid collisions with other apps
*and* with anything already listening on the host, starting from
`defaultExternal`. This is why PostgreSQL's `5432` inside often becomes `5433`
outside.

### `env[]`

Install-time inputs. Each entry becomes an environment variable passed to the
install script, and a field in the install dialog.

| Field | Type | Notes |
|---|---|---|
| `key` | string | The environment variable name. |
| `label` | string | Field label in the install dialog. Falls back to `key`. |
| `required` | bool | Reject the install if left blank with no default or generator. |
| `secret` | bool | Value is redacted in API responses and masked in the dialog. |
| `default` | string | Applied when the user leaves the field blank. |
| `generate` | string | `password` → a strong value is generated when blank. |

Resolution order for a blank field: `generate`, then `default`, then reject if
`required`.

### `connection`

Lets the UI show a uniform user/password/database panel for every server,
regardless of how the application names its variables.

| Field | Notes |
|---|---|
| `user` | Static username when there is no configurable one (MySQL's `root`). |
| `userEnv` | Env var holding the username. Takes precedence over `user`. |
| `passwordEnv` | Env var holding the password. |
| `databaseEnv` | Env var holding the default database, if any. |

### `ui`

Optional. Overrides what the SPA loads from the application's `ui/` directory. Every
path is relative to `ui/`.

| Field | Type | Default |
|---|---|---|
| `entry` | string | `scripts/main.js` if it exists |
| `styles` | string[] | every `.css` directly under `style/`, sorted |
| `views` | object | every `.html` directly under `views/`, keyed by filename without extension |

Omit the block entirely to take all three defaults — that is the common case,
and the layout convention makes it correct. See
[05 — Slots](05-slots.md) and [06 — Extension API](06-extension-api.md) for
what the entry module then does.

Every declared path must exist and must stay inside `ui/`. A typo fails
`NewRegistry()` at startup, not in someone's browser.

### `backend`

Optional, and only meaningful when the application ships a `backend/` directory —
which, exactly like `ui/`, is what opts the application in. There is nothing to name
here because the layout is fixed: the plugin is `backend/`, and it is
`package main`.

| Field | Type | Default | Notes |
|---|---|---|---|
| `access` | string | `registered` | `registered` — any signed-in user may call the plugin; `admin` — administrators only. |
| `timeoutMs` | int | `15000` | Bounds one call. A plugin that has not answered by then fails that call and keeps running. |

`access` is the only capability control the platform enforces on a plugin's
behalf. Anything finer is the plugin's own job, using `Request.Caller` — see
[15 — Backend plugins](15-backend-plugins.md).

An unknown backend `access` value or a negative `timeoutMs` fails `NewRegistry()`.

## Validation rules

Every application must declare a non-empty `version`. Loading fails without one —
including for an uploaded package, which is refused at upload rather than
half-added.

Enforced in `registry.go:validate` and `registry.go:loadApplication`:

- `id` must equal the directory name.
- `name` must not be empty.
- `version` must not be empty or whitespace.
- `scopes` must be non-empty and contain only `global` / `project`.
- An explicitly named `install` script must stay inside `infra/` and exist.
- `port`, `service`, host tools, and `healthcheck` require infrastructure.
- `defaultExternal` and `healthcheck` require `port.internal`.
- At least one of `infra/`, `backend/`, `ui/`, or `skills/` must contribute a capability.
- For `backend`: `port`, `service`, and `healthcheck` must all be absent, and a
  `backend/` directory must exist.
- Every path in the `ui` block must exist inside `ui/`.
- A `ui/` directory that exists must contain at least one file.
- A `backend/` directory that exists must contain at least one `package main`
  Go file, and must not contain its own `go.mod` or `go.sum` — the server
  generates those.

## Reserved directory names

`docs/` inside the catalog is this documentation, not an application. The registry
skips it (`registry.go:isCatalogMetadataDirectory`). Every other directory is loaded as an
application, so do not put anything else beside them.

## Skills

An application directory may carry a `skills/` directory. Like `ui/`, it opts the
application in by existing — nothing in `application.json` declares it. Each subdirectory
holding a `SKILL.md` is one skill:

```
skills/
  backup-ignore/
    SKILL.md
```

Names must be lowercase words joined by hyphens, because they become
directories in a project workspace. A subdirectory without a `SKILL.md` fails
the catalog load rather than shipping something the agent cannot read.

Installing a project-scoped application publishes each skill to
`/workspace/.agents/skills/<name>/SKILL.md` in that project's container, where
the agent picks it up alongside the platform's own skills; uninstalling removes
it again. Publishing is idempotent — a `.skill.sha256` marker beside the file
means an unchanged skill is not re-pushed. Global-scope apps have no project
workspace and publish nothing.

Ship a skill when using the application well requires knowledge the agent cannot
infer from the container, and keep it to what an agent needs to act. It is not
a place for user-facing documentation; that belongs in the application's README or
its UI.

## Host tools

Some applications need an executable on the **Remote host**, beside the server
process, rather than inside a container. Remote ships none of them and keeps no
package list: the application declares its own, and Remote downloads exactly that.

```json
"hostTools": [
  {
    "name": "restic",
    "version": "0.19.1",
    "downloads": {
      "amd64": {
        "url": "https://example.com/restic_0.19.1_linux_amd64.bz2",
        "sha256": "f415…585c",
        "compression": "bzip2"
      },
      "arm64": { "url": "…", "sha256": "…", "compression": "bzip2" }
    },
    "versionArgs": ["version"]
  }
]
```

* `downloads` is keyed by host architecture as Go names it (`amd64`, `arm64`).
  A host whose architecture is missing cannot install the application.
* `sha256` is the digest of the bytes at `url`, **before** decompression, and
  is mandatory. A download that hashes to anything else is discarded and the
  install fails; nothing is written to the host.
* `compression` is `""`, `"gzip"` or `"bzip2"` — one compressed executable, not
  an archive of several files. One application, one binary, one checksum to read.
* `versionArgs` (default `["version"]`) runs the installed binary to prove it
  works before the install is reported as successful.

Tools install under the server's data directory (`host-tools/<name>/<version>/`,
published as `host-tools/bin/<name>`), never into `/usr` and never through the
host package manager. Nothing outside Remote's own state is modified. Remote
prepares them before the guest install script runs, on both Install and Start,
and reuses an existing copy instead of downloading again. Only provisioned
applications may declare them; uninstall leaves them in place.

A host that installs no application declaring host tools downloads nothing, which is
what keeps such an application a genuinely optional addition rather than a dependency
every operator inherits.
