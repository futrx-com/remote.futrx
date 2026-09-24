# 12 — HTTP API

Every endpoint below sits behind the platform's session middleware: a valid
session for a registered user is required before any of them are reached.
Handlers then re-check authorization per route.

Routes are registered in
[`applications_handler.go`](../../../backend/internal/transport/http/handlers/applications_handler.go);
project-scoped routes are delegated there by `project_handler.go` after project
membership has been verified.

## Catalog

### `GET /api/applications/catalog`

Every application in the catalog. Readable by **any registered user**, because the
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
    "scopes": ["global", "project"],
    "port": { "internal": 3306, "defaultExternal": 3306, "protocol": "tcp",
              "bindAddress": "127.0.0.1" },
    "env": [ … ],
    "service": {
      "name": "mysql",
      "command": ["/usr/sbin/mysqld", "--port", "{{internalPort}}"]
    },
    "connection": { "user": "root", "passwordEnv": "MYSQL_ROOT_PASSWORD" },
    "install": "infra/install.sh",
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

Each entry also carries `"source"`: `"builtin"` for an application compiled into the
server, `"uploaded"` for one that came from an uploaded package. It is stamped
by the registry, never read from `application.json`.

### Packages

`GET`, `POST` and `DELETE` on `/api/applications/packages[/{id}]` manage
uploaded application packages. All admin-only; documented in full in
[16 — Uploaded packages](16-uploaded-packages.md#http).

### `GET /api/applications/catalog/{applicationID}/ui/{path}`

One file from an application's `ui/` directory. Any registered user; `GET` and `HEAD`.

```
GET /api/applications/catalog/mysql/ui/scripts/main.js
```

| Response header | Value |
|---|---|
| `Content-Type` | derived from the file extension (see below) |
| `X-Content-Type-Options` | `nosniff` |
| `Cache-Control` | `private, no-cache` |
| `ETag` | strong validator over the asset's bytes |

`no-cache` means "cache it, but revalidate", not "do not cache". A conditional
request answers `304` from memory. The assets are compiled into the binary, so
their URLs never change when their content does — a timed cache would serve a
stale extension for its whole lifetime after a rebuild, and for an ES module
the SPA imports, an author editing `ui/` would see their old code with nothing
to tell them why. Revalidating makes a rebuild visible on the next reload.

Types are pinned from the extension, never sniffed: `.js`/`.mjs` →
`text/javascript`, `.css` → `text/css`, `.html` → `text/html`, `.json` →
`application/json`, `.svg` → `application/svg+xml`, `.png`, `.webp`, `.woff2`, and
anything else → `application/octet-stream`.

Paths that escape the application's own `ui/` return **404**, as do files that do not
exist and applications with no `ui/`. The registry cleans and rejects the path before
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
    "application": { "id": "ui-playground", "name": "UI Playground", "ui": { … }, … },
    "global": true
  },
  {
    "application": { "id": "ui-sandbox", "name": "UI Sandbox", "ui": { … }, … },
    "global": false,
    "projectIds": ["20336ed6ab63"]
  },
  {
    "application": { "id": "backend-playground", "backend": { … },
               "backend": { "access": "registered", "timeoutMs": 10000 }, … },
    "global": true,
    "projectIds": ["20336ed6ab63"],
    "backends": [
      { "instanceId": "9f1c2ab40e77", "scope": "global" },
      { "instanceId": "3d5e81c6aa02", "scope": "project", "projectId": "20336ed6ab63" }
    ]
  }
]
```

`backends` lists the running backend processes the extension may call. An application
installed in several places runs one process per install, so this is what lets
`remote.backend` address the right one; it is absent for applications that ship no
`backend/`.

Included only when the application has a `ui/` **and** has a **running** instance
that is either global or in a project the caller can see. Visible projects come
from the project service's own `ListVisible(email, isAdmin)`, so this inherits
project membership exactly.

An application installed in several places appears once, with the union of its
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

## Backend backend routes

An instance whose application ships a `backend/` directory is reachable at a `backend`
sub-path. The bare prefix describes the backend; anything deeper is forwarded to
it verbatim.

| Method | Path | Does |
|---|---|---|
| `GET` | `/api/applications/{id}/backend` | Describe the backend |
| *any* | `/api/applications/{id}/backend/{path…}` | Call the backend |
| `GET` | `/api/projects/{projectID}/applications/{id}/backend` | Describe |
| *any* | `/api/projects/{projectID}/applications/{id}/backend/{path…}` | Call |
| `GET` | `/api/chats/{chatID}/applications/{id}/backend` | Describe after authorizing the chat and install scope |
| *any* | `/api/chats/{chatID}/applications/{id}/backend/{path…}` | Call with trusted chat/workspace context |

Calling a backend on a **global** instance is the one action there that is not
admin-only. The backend is the server side of an extension that renders for
every signed-in user, so managing the app stays admin-only while calling it
requires only a session — narrowed to administrators when the application declares
`"backend": { "access": "admin" }`. Project routes require membership, checked
before delegation as everywhere else.

The chat route first applies the same session and project-membership check as
every other chat resource. It then allows a global instance or an instance
installed in that chat's project and stamps this server-resolved context onto
the forwarded request:

```json
{
  "context": {
    "chat": {
      "id": "chat-123",
      "projectId": "project-456",
      "workspaceRoot": "/var/lib/remote/projects/example/workspace"
    }
  }
}
```

Loose chats omit `projectId`. Their chat route can therefore address only a
global instance. Browser-supplied context is discarded on every route.

### `GET …/backend`

```json
{
  "instanceId": "9f1c2ab40e77",
  "applicationId": "backend-playground",
  "descriptor": {
    "name": "Backend Playground",
    "version": "1",
    "apiVersion": 1,
    "routes": [
      { "method": "GET", "path": "health", "description": "Process identity and uptime" }
    ]
  },
  "access": "registered",
  "timeoutMs": 10000
}
```

### `… /backend/{path…}`

The request is forwarded with its method, path, query, body, and headers. Two
things are **not** forwarded: `Cookie` and `Authorization`. The caller is
supplied separately, resolved from the session, so a backend can authorize a
caller without being able to act as them.

Request bodies are capped at 1 MiB.

The backend's answer becomes the HTTP response as-is, minus `Set-Cookie` and
hop-by-hop headers, and always with `X-Content-Type-Options: nosniff`. A backend
that sets no status answers `200`; one that sets no content type answers
`application/octet-stream`.

```
POST /api/applications/9f1c2ab40e77/backend/kv/greeting
Content-Type: application/json

{"value":"hello"}
```

```json
{ "key": "greeting", "value": "hello" }
```

The full contract is [15 — Application backends](15-application-backends.md).

## Payloads

### Install

```json
POST /api/applications
{
  "applicationId": "mysql",
  "name": "Primary database",
  "env": { "MYSQL_DATABASE": "app" },
  "externalPort": 3307,
  "bindAddress": "127.0.0.1"
}
```

Every field except `applicationId` is optional. A blank `env` value takes the application's
`generate` or `default`; a blank `externalPort` is allocated automatically.
Responds `201` with the instance view.

For a `ui` application, `externalPort` and `bindAddress` are ignored — there is no
port.

### Change port

```json
PUT /api/applications/{id}/port
{ "port": 3307 }
```

Rejected for `ui` applications: they have no port.

## Response shapes

**Instance view** — secret env values are redacted; `envPublic` holds the
non-secret ones:

```json
{
  "id": "abc123", "applicationId": "mysql", "name": "MySQL",
  "scope": "global", "projectId": "",
  "containerName": "futrx-app-abc123", "deviceName": "app-abc123",
  "internalPort": 3306, "externalPort": 3307,
  "bindAddress": "127.0.0.1", "protocol": "tcp",
  "status": "running", "error": "",
  "createdAt": 1788512000, "updatedAt": 1788512000,
  "envPublic": { "MYSQL_DATABASE": "app" }
}
```

`status` is one of `installing`, `running`, `stopped`, `error`. A `ui` application's
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
| `400` | unknown application, unsupported scope, missing project id, missing required env, port out of range |
| `401` | no valid session |
| `403` | admin-only route, non-admin caller |
| `403` | an `access: admin` backend and a non-admin caller |
| `404` | unknown instance, wrong scope for the route, asset not found or out of bounds, the application ships no backend |
| `405` | wrong method |
| `409` | this application is already installed in this scope; a backend call while the app is stopped |
| `500` | anything else, including install-script failure, a backend that failed to compile, and a call that timed out |
| `503` | applications unavailable (no container runtime configured), or no backend host |

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
