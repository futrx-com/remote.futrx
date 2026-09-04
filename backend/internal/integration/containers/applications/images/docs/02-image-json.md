# 02 — `image.json` reference

Every image directory contains exactly one `image.json`. It is loaded and
validated at server startup by `registry.go:loadImage`; a malformed file fails
the build and the tests rather than producing a broken catalog entry.

The Go type behind it is `Image` in
[`service/applications/model.go`](../../../../../service/applications/model.go).

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
| `id` | string | yes | Must equal the directory name. Loading fails otherwise. |
| `name` | string | yes | Display name in the catalog and on installed rows. |
| `description` | string | no | One line; the card truncates to two lines. |
| `category` | string | no | Free text, e.g. `database`, `cache`, `development`. |
| `version` | string | no | Shown next to the name. A string, not a number — `"8.0"`, `"16"`. |
| `icon` | string | no | Built-in key or a path into this image's `ui/`. See [09 — Styling and icons](09-styling-and-icons.md). |
| `type` | string | no | `service` (default) or `ui`. See [03 — Image types](03-image-types.md). |
| `scopes` | string[] | yes | Any of `global`, `project`. At least one. |
| `base` | string | no | LXD image for a dedicated global container. Default `ubuntu:24.04`. `service` only. |
| `port` | object | for `service` | See below. Forbidden on `ui`. |
| `env` | object[] | no | Install-time inputs. See below. |
| `service` | string | no | systemd unit name inside the container. Forbidden on `ui`. |
| `connection` | object | no | Maps env vars to user/password/database. See below. |
| `install` | string | no | Install-script filename. Default `install.sh`. Ignored for `ui`. |
| `healthcheck` | object | no | `{ "command": "…" }` run inside the container. Forbidden on `ui`. |
| `ui` | object | no | Overrides what is loaded from `ui/`. See below. |

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

## Validation rules

Enforced in `registry.go:validate` and `registry.go:loadImage`:

- `id` must equal the directory name.
- `name` must not be empty.
- `type` must be `service`, `ui`, or absent (which means `service`).
- `scopes` must be non-empty and contain only `global` / `project`.
- For `service`: `port.internal` must be > 0, and the install script named by
  `install` must exist.
- For `ui`: `port`, `service`, and `healthcheck` must all be absent, and a
  `ui/` directory must exist.
- Every path in the `ui` block must exist inside `ui/`.
- A `ui/` directory that exists must contain at least one file.

## Reserved directory names

`docs/` inside the catalog is this documentation, not an image. The registry
skips it (`registry.go:isCatalogMetadataDirectory`). Every other directory is loaded as an
image, so do not put anything else beside them.
