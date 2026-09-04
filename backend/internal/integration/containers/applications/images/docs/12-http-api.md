# 12 — HTTP API

Every endpoint below sits behind the platform's session middleware: a valid
session for a registered user is required before any of them are reached.
Handlers then re-check authorization per route.

Routes are registered in
[`applications_handler.go`](../../../../../transport/http/handlers/applications_handler.go);
project-scoped routes are delegated there by `project_handler.go` after project
membership has been verified.

## Catalog

### `GET /api/applications/catalog`

Every image in the catalog. Readable by **any registered user**, because the
project UI uses the same catalog.

```json
[
  {
    "id": "mysql",
    "name": "MySQL",
    "description": "Popular open-source relational database server.",
    "category": "database",
    "version": "8.0",
    "icon": "database",
    "type": "service",
    "scopes": ["global", "project"],
    "port": { "internal": 3306, "defaultExternal": 3306, "protocol": "tcp",
              "bindAddress": "127.0.0.1" },
    "env": [ … ],
    "service": "mysql",
    "connection": { "user": "root", "passwordEnv": "MYSQL_ROOT_PASSWORD" },
    "install": "install.sh",
    "ui": {
      "entry": "scripts/main.js",
      "styles": ["style/popup.css", "style/style.css"],
      "views": { "index": "views/index.html", "popup": "views/popup.html" }
    }
  }
]
```

This lists what *can* be installed. It grants nothing — see
`/api/applications/ui` below.

### `GET /api/applications/catalog/{imageID}/ui/{path}`

One file from an image's `ui/` directory. Any registered user; `GET` and `HEAD`.

```
GET /api/applications/catalog/mysql/ui/scripts/main.js
```

| Response header | Value |
|---|---|
| `Content-Type` | derived from the file extension (see below) |
| `X-Content-Type-Options` | `nosniff` |
| `Cache-Control` | `private, max-age=300` |

Types are pinned from the extension, never sniffed: `.js`/`.mjs` →
`text/javascript`, `.css` → `text/css`, `.html` → `text/html`, `.json` →
`application/json`, `.svg` → `image/svg+xml`, `.png`, `.webp`, `.woff2`, and
anything else → `application/octet-stream`.

Paths that escape the image's own `ui/` return **404**, as do files that do not
exist and images with no `ui/`. The registry cleans and rejects the path before
any read; see [13 — Security model](13-security-model.md).

The five-minute cache is why a hard reload helps while iterating on extension
assets.

## Which extensions to load

### `GET /api/applications/ui`

The list this caller is allowed to load, with each entry's install scope. Any
registered user.

```json
[
  {
    "image": { "id": "ui-playground", "name": "UI Playground", "type": "ui", … },
    "global": true
  },
  {
    "image": { "id": "ui-sandbox", "name": "UI Sandbox", "type": "ui", … },
    "global": false,
    "projectIds": ["20336ed6ab63"]
  }
]
```

Included only when the image has a `ui/` **and** has a **running** instance
that is either global or in a project the caller can see. Visible projects come
from the project service's own `ListVisible(email, isAdmin)`, so this inherits
project membership exactly.

An image installed in several places appears once, with the union of its
scopes. Returns `[]` when the applications service is unavailable, so the SPA
degrades to no extensions rather than erroring.

This is the endpoint the extension host calls; the catalog endpoint is not used
for loading.

## Global instances — admin only

| Method | Path | Does |
|---|---|---|
| `GET` | `/api/applications` | List global instances |
| `POST` | `/api/applications` | Install at global scope |
| `GET` | `/api/applications/{id}` | One instance |
| `DELETE` | `/api/applications/{id}` | Uninstall |
| `GET` | `/api/applications/{id}/credentials` | Full connection detail, secrets included |
| `POST` | `/api/applications/{id}/start` | Start |
| `POST` | `/api/applications/{id}/stop` | Stop |
| `PUT` | `/api/applications/{id}/port` | Change the host port |

Non-admins get `403 admin only`, because a global app is server-wide
infrastructure. An id belonging to a project instance is reported as `404` on
these routes, so a project app cannot be controlled through them.

## Project instances

Identical set, under a project, available to any **member** of that project:

| Method | Path |
|---|---|
| `GET` | `/api/projects/{projectID}/applications` |
| `POST` | `/api/projects/{projectID}/applications` |
| `GET` | `/api/projects/{projectID}/applications/{id}` |
| `DELETE` | `/api/projects/{projectID}/applications/{id}` |
| `GET` | `/api/projects/{projectID}/applications/{id}/credentials` |
| `POST` | `/api/projects/{projectID}/applications/{id}/start` |
| `POST` | `/api/projects/{projectID}/applications/{id}/stop` |
| `PUT` | `/api/projects/{projectID}/applications/{id}/port` |

Membership is checked by `project_handler.go` before delegating. The handler
then verifies the instance actually belongs to *this* project, so a member of
one project cannot control another's app by guessing its id.

## Payloads

### Install

```json
POST /api/applications
{
  "imageId": "mysql",
  "name": "Primary database",
  "env": { "MYSQL_DATABASE": "app" },
  "externalPort": 3307,
  "bindAddress": "127.0.0.1"
}
```

Every field except `imageId` is optional. A blank `env` value takes the image's
`generate` or `default`; a blank `externalPort` is allocated automatically.
Responds `201` with the instance view.

For a `ui` image, `externalPort` and `bindAddress` are ignored — there is no
port.

### Change port

```json
PUT /api/applications/{id}/port
{ "port": 3307 }
```

Rejected for `ui` images: they have no port.

## Response shapes

**Instance view** — secret env values are redacted; `envPublic` holds the
non-secret ones:

```json
{
  "id": "abc123", "imageId": "mysql", "name": "MySQL",
  "scope": "global", "projectId": "",
  "containerName": "futrx-app-abc123", "deviceName": "app-abc123",
  "internalPort": 3306, "externalPort": 3307,
  "bindAddress": "127.0.0.1", "protocol": "tcp",
  "status": "running", "error": "",
  "createdAt": 1788512000, "updatedAt": 1788512000,
  "envPublic": { "MYSQL_DATABASE": "app" }
}
```

`status` is one of `installing`, `running`, `stopped`, `error`. A `ui` image's
instance has `internalPort: 0`, `externalPort: 0`, and an empty
`containerName`.

**Credentials** — returned only from the credentials route, which the transport
has authorized:

```json
{
  "containerName": "futrx-app-abc123",
  "lxdHost": "futrx-app-abc123.lxd",
  "internalPort": 3306, "externalPort": 3307, "bindAddress": "127.0.0.1",
  "username": "root", "password": "…", "database": "app",
  "env": { … }
}
```

## Errors

| Status | When |
|---|---|
| `400` | unknown image, unsupported scope, missing project id, missing required env, port out of range |
| `401` | no valid session |
| `403` | admin-only route, non-admin caller |
| `404` | unknown instance, wrong scope for the route, asset not found or out of bounds |
| `405` | wrong method |
| `409` | this image is already installed in this scope |
| `500` | anything else, including install-script failure |
| `503` | applications unavailable (no container runtime configured) |

Bodies are `{"error": "…"}`. An install-script failure includes the tail of the
script's output, which is what the UI shows on the instance row.

## Calling these from an extension

Use `fetch` with `credentials: "same-origin"` — you are the signed-in user,
with exactly their authorization. An extension in a non-admin's browser cannot
list global instances, so prefer the project routes when you have a
`context.projectId`:

```js
const path = context.projectId
  ? `/api/projects/${encodeURIComponent(context.projectId)}/applications`
  : "/api/applications";
const response = await fetch(path, { credentials: "same-origin" });
```
