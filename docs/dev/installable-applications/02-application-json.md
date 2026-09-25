# 02 — `application.json` reference

Every application directory contains exactly one `application.json`. Built-in
entries are loaded and validated at server startup by
`registry.go:loadApplication`; uploaded entries pass the same loader before
the package is accepted. A malformed file is refused rather than producing a
broken catalog entry.

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
  "service": {
    "name": "mysql",
    "description": "MySQL database server",
    "command": ["/usr/sbin/mysqld", "--port", "{{internalPort}}"],
    "user": "mysql",
    "group": "mysql",
    "restart": "on-failure",
    "restartSec": 1,
    "environment": [
      {"key": "MYSQL_ROOT_PASSWORD_B64", "fromEnv": "MYSQL_ROOT_PASSWORD", "encoding": "base64"}
    ],
    "hardening": {
      "noNewPrivileges": true,
      "privateTmp": true,
      "protectHome": true,
      "protectSystem": "full"
    }
  },
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
  "description": "Developer fixture: a Go backend that exercises every part of the backend API.",
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

An application backend that publishes its own events and consumes both Remote
lifecycle events and another application's events:

```json
{
  "id": "event-worker",
  "name": "Event Worker",
  "version": "1",
  "scopes": ["global", "project"],
  "publishers": [
    {
      "name": "jobs",
      "events": [
        {
          "name": "completed",
          "version": 1,
          "description": "A job completed successfully."
        }
      ]
    }
  ],
  "subscriptions": [
    {
      "publisher": "remote.applications",
      "events": ["installed", "uninstalled", "started", "stopped"]
    },
    {
      "publisher": "applications.other-app.imports",
      "events": ["completed"]
    }
  ]
}
```

This package must also contain a host backend: event declarations without one
are rejected. Keep typed business-event triggers and subscriber behavior in
the importable child package `backend/lifecycle/`. `backend/main.go` receives
the core-owned event runtime and composes those dependencies; it does not
implement a publisher. All child packages compile into the same process. See
[18 — Backend event lifecycle](18-application-events.md) for the runtime API,
routing, and delivery guarantees.

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
  "service": {
    "name": "object-mount",
    "command": ["/usr/local/bin/object-mount"]
  },
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
| `version` | string | **yes** | A string, not a number — `"8.0"`, `"16"`, `"1.2.3-rc1"`. Shown next to the name, and the signal that reconverges an installed copy's container programs, custom installer, and manifest service when it changes. See [17 — Versions and upgrades](17-versions-and-upgrades.md). |
| `icon` | string | no | Built-in key or a path into this application's `ui/`. See [09 — Styling and icons](09-styling-and-icons.md). |
| `scopes` | string[] | yes | Any of `global`, `project`. At least one. |
| `base` | string | no | LXD image for a dedicated global infrastructure container. Default `ubuntu:24.04`. |
| `port` | object | no | See below. Requires infrastructure; omit it when nothing is exposed. |
| `env` | object[] | no | Install-time inputs. See below. |
| `service` | object | no | Complete systemd service declaration. It is itself a container capability; Remote creates and owns the unit. See below. |
| `connection` | object | no | Maps env vars to user/password/database. See below. |
| `install` | string | no | Override for the install-script path inside `infra/`. When omitted, `infra/install.sh` is detected automatically. |
| `healthcheck` | object | no | `{ "command": "…" }` run inside the container. Requires `port.internal`. |
| `ui` | object | no | Overrides what is loaded from `ui/`. See below. |
| `backend` | object | no | Overrides the defaults for the Go backend whose executable entry point is `backend/main.go`. See below. |
| `publishers` | object[] | no | Event families the backend may publish. Publisher names are local; Remote qualifies them as `applications.<application-id>.<publisher>`. See below. |
| `subscriptions` | object[] | no | Canonically named event families delivered to running backend instances. See below. |
| `source` | string | — | **Response-only.** The server sets `builtin` or `uploaded`; declaring this field in `application.json` is rejected. |

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

### `service`

Declares a supervised container process without asking an install script to
write systemd files. Remote writes the environment and unit, registers the
daemon with workspace-idle detection, reloads systemd, enables and restarts the
unit, and owns start/stop/uninstall lifecycle.

| Field | Type | Notes |
|---|---|---|
| `name` | string | Unit name without `.service`. |
| `description` | string | One-line systemd description; defaults to the application name. |
| `command` | string[] | Absolute executable followed by arguments. `{{internalPort}}` is replaced with the instance's internal port. |
| `user`, `group` | string | Optional service identity. Omit both to run as root. |
| `restart` | string | One of systemd's standard restart policies: `no`, `on-success`, `on-failure`, `on-abnormal`, `on-watchdog`, `on-abort`, or `always`. |
| `restartSec` | int | Non-negative restart delay in seconds. |
| `environment` | object[] | Maps a declared `env[]` key into the service environment. Each entry is `{ "key", "fromEnv", "encoding": "base64" }`. |
| `hardening` | object | Optional `noNewPrivileges`, `privateTmp`, `protectHome`, and `protectSystem` (`true`, `full`, or `strict`). |

Environment mappings are deliberately base64 encoded. This preserves spaces,
line breaks, quotes, and secrets without letting a value change systemd's
environment-file syntax. The service decodes those values itself, as Hello
Remote does for its `HELLO_*_B64` variables.

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

Optional, and only meaningful when the application ships a host backend. In
the current layout, the `backend/` root is the required executable
`package main` and composition root. Child directories such as `backend/api/`
and `backend/lifecycle/` are ordinary importable packages in that same
generated host module. They organize implementation ownership; they do not opt
the application into an additional capability or process. A `package main`
under `backend/api/` remains accepted for older uploaded packages.

| Field | Type | Default | Notes |
|---|---|---|---|
| `access` | string | `registered` | `registered` — any signed-in user may call the backend; `admin` — administrators only. |
| `timeoutMs` | int | `15000` | Requests the bound for one call; `0` selects the default. Any nonnegative value is accepted for compatibility, but the effective runtime maximum is `300000` (five minutes). A backend that has not answered by then fails that call and keeps running. Event delivery has a separate 30-second maximum. |

`access` is the only capability control the platform enforces on a backend's
behalf. Anything finer is the backend's own job, using `Request.Caller` — see
[15 — Application backends](15-application-backends.md).

An unknown backend `access` value or a negative `timeoutMs` fails
`NewRegistry()`.

### `publishers[]`

Declares exactly what an application's backend is allowed to emit. The
publisher `name` is local to the application manifest; for example, `jobs` on
application `event-worker` is delivered under the canonical publisher
`applications.event-worker.jobs`.

| Field | Type | Notes |
|---|---|---|
| `name` | string | Local lowercase name, at most 128 bytes, made of alphanumeric segments separated by `.` or `-`. It cannot begin with the reserved `remote` or `applications` segment. |
| `events` | object[] | Between 1 and 128 event declarations; names must be unique within this publisher. |

Each event declaration has:

| Field | Type | Notes |
|---|---|---|
| `name` | string | At most 128 bytes; lowercase alphanumeric segments separated by `.` or `-`. |
| `version` | int | Payload schema version, starting at `1`. Change it when the payload contract changes. |
| `description` | string | Optional one-line description, at most 2048 bytes. |

Publication is checked against all three manifest values: local publisher,
event name, and version. Declaring a publisher does not publish anything by
itself. Core constructs `Runtime.Events` from the installed manifest and binds
it before `Backend.Init`; application code does not implement or initialize a
publisher. Conventionally a typed wrapper in `backend/lifecycle/` calls
`EventEmitter.Emit` wherever the application's business logic recognizes that
an event occurred. Every emission is checked again against the installed
manifest before core stamps its source and dispatches it.

### `subscriptions[]`

Selects events for delivery to a running backend instance.

| Field | Type | Notes |
|---|---|---|
| `publisher` | string | Canonical publisher, at most 256 bytes: `remote.applications`, or `applications.<application-id>.<local-publisher>`. |
| `events` | string[] | Between 1 and 128 unique event names, each at most 128 bytes. Subscriptions select names, not versions; inspect `Event.Version` in the handler. |

Only one subscription block may name a particular canonical publisher. The
seven valid names for `remote.applications` are `added`, `updated`, `deleted`,
`installed`, `uninstalled`, `started`, and `stopped`. An application-owned
publisher is validated structurally here; its existence and event list may
change independently with that package, so subscribers must handle versions
and payloads defensively.

Like `publishers`, subscriptions require a host backend. A manifest declaration
does not silently turn an ordinary backend into a subscriber: the value served
by the root composition package must implement `applications.EventSubscriber`, or
its install or start fails during the backend handshake. Put the cohesive
implementation in `backend/lifecycle/`; embedding or delegating to it from the
served value keeps the event contract out of the HTTP API package without
creating a second process.

## Validation rules

Every application must declare a non-empty `version`. Loading fails without one —
including for an uploaded package, which is refused at upload rather than
half-added.

`application.json` is limited to 256 KiB. Field names are exact and
case-sensitive; unknown fields, duplicate object keys, and trailing JSON values
are rejected. `source`, `container`, and `skills` are derived from the catalog
layout and cannot be declared in the manifest.

That strict decoder applies to built-in applications and every new or
replacement upload. Packages already persisted by an older Remote release are
first try the strict current schema, then fall back to a frozen pre-events
`encoding/json` schema. That keeps an installed application from disappearing
merely because its old manifest contained an ignored, case-aliased, or
response-only field. It also prevents a formerly unknown field named
`publishers` or `subscriptions` from unexpectedly activating a capability.
All recognized legacy values still pass current validation, and Remote still
recomputes `source`, `container`, and `skills`. Re-uploading such a package
requires its manifest to satisfy the strict current contract.

Enforced in `registry_validation.go:validateApplication` and
`registry.go:loadApplication`:

- `id` must equal the directory name.
- `name` must not be empty.
- `version` must not be empty or whitespace.
- `scopes` must be non-empty and contain only `global` / `project`.
- An explicitly named `install` script must stay inside `infra/` and exist.
- `port`, host tools, and `healthcheck` require a container capability. A
  `service` declaration is itself such a capability.
- A service requires a valid unit `name` and an absolute executable in
  `command`; its environment mappings must reference declared `env[]` keys.
- `defaultExternal` and `healthcheck` require `port.internal`.
- At least one of `infra/`, `backend/`, `ui/`, or `skills/` must contribute a capability.
- A declared `backend` block requires an executable at `backend/main.go` (or
  the legacy `backend/api/` executable layout). Backend, infrastructure,
  service, port, and health-check capabilities may coexist in one application.
- `publishers` and `subscriptions` require host backend source. Publisher names
  are local and unique, events are unique per publisher and have versions of
  at least `1`, and subscriptions use canonical publisher names with unique
  event names.
- A manifest may declare at most 64 publishers and 128 subscriptions. Each
  publisher or subscription may list at most 128 events; publisher names,
  canonical publisher names, event names, and event descriptions have the
  byte limits documented above.
- `remote.applications` subscriptions may name only its seven documented
  version-1 lifecycle events.
- Every path in the `ui` block must exist inside `ui/`.
- A `ui/` directory that exists must contain at least one file.
- In the current layout, the `backend/` root must contain at least one non-test
  `package main` Go file. Child host directories such as `backend/api/` and
  `backend/lifecycle/` are importable packages compiled into that executable.
- The host backend tree must not contain `go.mod`, `go.sum`, `go.work`, or
  `go.work.sum`; Remote generates `go.mod` and owns the module/workspace
  resolution used to build `.`. `backend/container/` is excluded from the
  host tree and may carry module control files for its separate in-container
  build.

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
