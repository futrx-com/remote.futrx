# 02 — `image.json` reference

Every image directory contains exactly one `image.json`. It is loaded and
validated at server startup by `registry.go:loadImage`; a malformed file fails
the build and the tests rather than producing a broken catalog entry.

The Go type behind it is `Image` in
[`service/applications/model.go`](../../../backend/internal/service/applications/model.go).

## Complete example

A service image using every relevant field:

```json
{
  "id": "mysql",
  "name": "MySQL",
  "description": "Popular open-source relational database server.",
  "category": "database",
  "version": "8.0",
  "icon": "database",
  "type": "service",
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
  "install": "install.sh",
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

A backend image, which needs almost nothing beyond its `plugin/` directory:

```json
{
  "id": "backend-playground",
  "name": "Backend Playground",
  "description": "Developer fixture: a Go plugin that exercises every part of the backend API.",
  "category": "development",
  "version": "1",
  "icon": "ui/assets/logo.svg",
  "type": "backend",
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

A tool image, which provisions into the project's container but exposes
nothing — no port, no healthcheck, project scope only:

```json
{
  "id": "object-mount",
  "name": "Object Mount",
  "description": "Mount a bucket as a normal filesystem inside this project's container.",
  "category": "storage",
  "version": "0.1.0",
  "icon": "disk",
  "type": "tool",
  "scopes": ["project"],
  "env": [
    { "key": "MOUNT_BUCKET", "label": "Bucket", "required": true },
    { "key": "MOUNT_POINT", "label": "Mount at", "default": "/workspace/bucket" },
    { "key": "AWS_ACCESS_KEY_ID", "label": "Access key ID", "required": true, "secret": true },
    { "key": "AWS_SECRET_ACCESS_KEY", "label": "Secret access key", "required": true, "secret": true }
  ],
  "service": "object-mount",
  "install": "install.sh"
}
```

A UI image, which needs far less:

```json
{
  "id": "ui-playground",
  "name": "UI Playground",
  "description": "Developer fixture: contributes to every extension slot.",
  "category": "development",
  "version": "1",
  "icon": "ui/assets/logo.svg",
  "type": "ui",
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
| `version` | string | **yes** | A string, not a number — `"8.0"`, `"16"`, `"1.2.3-rc1"`. Shown next to the name, and the signal that re-runs `install.sh` on an installed copy when it changes. See [17 — Versions and upgrades](17-versions-and-upgrades.md). |
| `icon` | string | no | Built-in key or a path into this image's `ui/`. See [09 — Styling and icons](09-styling-and-icons.md). |
| `type` | string | no | `service` (default), `tool`, `ui`, or `backend`. See [03 — Image types](03-image-types.md). |
| `scopes` | string[] | yes | Any of `global`, `project`. At least one. |
| `base` | string | no | LXD image for a dedicated global container. Default `ubuntu:24.04`. `service` only. |
| `port` | object | for `service` | See below. Forbidden on `tool`, `ui` and `backend`, none of which is reachable on a port. |
| `env` | object[] | no | Install-time inputs. See below. |
| `service` | string | no | systemd unit name inside the container. Meaningful for `service` and `tool` — it is what stop and uninstall act on. Forbidden on `ui` and `backend`, which have no container. |
| `connection` | object | no | Maps env vars to user/password/database. See below. |
| `install` | string | no | Install-script filename. Default `install.sh`. Required for `service` and `tool`; ignored for `ui` and `backend`. |
| `healthcheck` | object | no | `{ "command": "…" }` run inside the container. It probes a port, so it is forbidden on `tool`, `ui` and `backend`. |
| `ui` | object | no | Overrides what is loaded from `ui/`. See below. |
| `backend` | object | no | Overrides the defaults for the Go plugin in `plugin/`. See below. |
| `source` | string | — | **Server-set, not accepted from `image.json`.** `builtin` or `uploaded`; anything declared here is overwritten. |

### `port`

Describes how a `service` image exposes itself. Required when `type` is
`service` (or omitted); rejected when `type` is `ui`.

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
regardless of how the image names its variables.

| Field | Notes |
|---|---|
| `user` | Static username when there is no configurable one (MySQL's `root`). |
| `userEnv` | Env var holding the username. Takes precedence over `user`. |
| `passwordEnv` | Env var holding the password. |
| `databaseEnv` | Env var holding the default database, if any. |

### `ui`

Optional. Overrides what the SPA loads from the image's `ui/` directory. Every
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

Optional, and only meaningful when the image ships a `plugin/` directory —
which, exactly like `ui/`, is what opts the image in. There is nothing to name
here because the layout is fixed: the plugin is `plugin/`, and it is
`package main`.

| Field | Type | Default | Notes |
|---|---|---|---|
| `access` | string | `registered` | `registered` — any signed-in user may call the plugin; `admin` — administrators only. |
| `timeoutMs` | int | `15000` | Bounds one call. A plugin that has not answered by then fails that call and keeps running. |

`access` is the only capability control the platform enforces on a plugin's
behalf. Anything finer is the plugin's own job, using `Request.Caller` — see
[15 — Backend plugins](15-backend-plugins.md).

Declaring `backend` without a `plugin/` directory fails `NewRegistry()`, as
does an unknown `access` value or a negative `timeoutMs`.

## Validation rules

Every image must declare a non-empty `version`. Loading fails without one —
including for an uploaded package, which is refused at upload rather than
half-added.

Enforced in `registry.go:validate` and `registry.go:loadImage`:

- `id` must equal the directory name.
- `name` must not be empty.
- `version` must not be empty or whitespace.
- `type` must be `service`, `tool`, `ui`, `backend`, or absent (which means
  `service`).
- `scopes` must be non-empty and contain only `global` / `project`.
- For `service`: `port.internal` must be > 0, and the install script named by
  `install` must exist.
- For `tool`: `port` and `healthcheck` must be absent, `scopes` must not
  contain `global`, and the install script named by `install` must exist.
- For `ui`: `port`, `service`, and `healthcheck` must all be absent, and a
  `ui/` directory must exist.
- For `backend`: `port`, `service`, and `healthcheck` must all be absent, and a
  `plugin/` directory must exist.
- Every path in the `ui` block must exist inside `ui/`.
- A `ui/` directory that exists must contain at least one file.
- A `plugin/` directory that exists must contain at least one `package main`
  Go file, and must not contain its own `go.mod` or `go.sum` — the server
  generates those.

## Reserved directory names

`docs/` inside the catalog is this documentation, not an image. The registry
skips it (`registry.go:isCatalogMetadataDirectory`). Every other directory is loaded as an
image, so do not put anything else beside them.

## Skills

An image directory may carry a `skills/` directory. Like `ui/`, it opts the
image in by existing — nothing in `image.json` declares it. Each subdirectory
holding a `SKILL.md` is one skill:

```
skills/
  backup-ignore/
    SKILL.md
```

Names must be lowercase words joined by hyphens, because they become
directories in a project workspace. A subdirectory without a `SKILL.md` fails
the catalog load rather than shipping something the agent cannot read.

Installing a project-scoped image publishes each skill to
`/workspace/.agents/skills/<name>/SKILL.md` in that project's container, where
the agent picks it up alongside the platform's own skills; uninstalling removes
it again. Publishing is idempotent — a `.skill.sha256` marker beside the file
means an unchanged skill is not re-pushed. Global-scope apps have no project
workspace and publish nothing.

Ship a skill when using the image well requires knowledge the agent cannot
infer from the container, and keep it to what an agent needs to act. It is not
a place for user-facing documentation; that belongs in the image's README or
its UI.

## Host tools

Some images need an executable on the **Remote host**, beside the server
process, rather than inside a container. Remote ships none of them and keeps no
package list: the image declares its own, and Remote downloads exactly that.

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
  A host whose architecture is missing cannot install the image.
* `sha256` is the digest of the bytes at `url`, **before** decompression, and
  is mandatory. A download that hashes to anything else is discarded and the
  install fails; nothing is written to the host.
* `compression` is `""`, `"gzip"` or `"bzip2"` — one compressed executable, not
  an archive of several files. One image, one binary, one checksum to read.
* `versionArgs` (default `["version"]`) runs the installed binary to prove it
  works before the install is reported as successful.

Tools install under the server's data directory (`host-tools/<name>/<version>/`,
published as `host-tools/bin/<name>`), never into `/usr` and never through the
host package manager. Nothing outside Remote's own state is modified. Remote
prepares them before the guest install script runs, on both Install and Start,
and reuses an existing copy instead of downloading again. Only provisioned
images may declare them; uninstall leaves them in place.

A host that installs no image declaring host tools downloads nothing, which is
what keeps such an image a genuinely optional addition rather than a dependency
every operator inherits.

