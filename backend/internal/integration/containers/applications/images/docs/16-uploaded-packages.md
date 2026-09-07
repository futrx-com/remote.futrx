# 16 — Uploaded packages

Everything else in this documentation describes images that ship **inside the
server binary**: a directory under `images/`, embedded at build time, released
when Remote is released.

An **uploaded package** is the same thing, delivered at runtime. An
administrator uploads a `.zip` from **Settings → Applications**; from that
moment it is an ordinary catalog entry. It appears under *Available
applications*, it installs at the scopes it declares, its `ui/` loads in the
browser, its `plugin/` is compiled and run. Nothing downstream of the registry
can tell the difference, because nothing downstream is told.

The one thing that differs is where it lives, and that is the point.

## Where a package lives, and why it survives an update

Uploaded packages are written to the server's **state directory**
(`DATA_DIR`, `/opt/remote.futrx/data` by default), never into the checkout and
never into the binary:

```
$DATA_DIR/app-packages/
  images/<id>/        the extracted package — exactly an images/<id>/ directory
  meta/<id>.json      who uploaded it, when, from which archive
  staging/            half-written uploads; never read
```

Updating Remote replaces `/opt/remote.futrx` and the binary in it. It does not
touch `$DATA_DIR`. So an update replaces the program and its built-in catalog
and leaves every uploaded application in place — along with the instances
installed from it and their settings, which were already stored beside it under
`$DATA_DIR`.

The layout is chosen so the directory **is** a catalog filesystem: `os.DirFS`
over `app-packages/` has the same `images/<id>/` shape the embedded catalog
has, and loads through the same `loadImage` the built-in one does. There is no
second loader to keep in step, and no validation a package can pass that a
built-in image would not.

## What a package archive holds

A `.zip` of exactly what an `images/<id>/` directory holds:

```
image.json          required — and it must set "id" and "version"
install.sh          for a service or tool
container.tar.gz    optional container payload (see 04 — Install scripts)
ui/…                optional browser extension
plugin/…            optional Go backend
```

Both shapes are accepted: the files at the archive root, or inside a single
folder — which is what "compress this folder" produces on a desktop. macOS
bookkeeping (`__MACOSX/`, `.DS_Store`, `._*`) is ignored.

So any zip tool will do — unless the image carries a `container.tar.gz`, which
nothing here builds for you: the archive is extracted as it arrives, and an
image whose container source is a nested Go module has to ship the payload
already packed. [`s3disk/package.sh`](../s3disk/package.sh) is the worked
example, and `--zip` writes the whole archive.

`image.json` must set both `"id"` and `"version"`.

`"id"` has to be explicit here. A built-in image may omit it and inherit its
directory name; an uploaded one has no directory to inherit from until the id
is known, and deriving it from the uploaded filename would let the same package
install under two ids depending on what the browser called the file.

`"version"` is required of every image, built-in or uploaded, because it is
what decides whether a later upload re-runs the install script on copies people
already have. See
[17 — Versions and upgrades](17-versions-and-upgrades.md).

The id must match `^[a-z0-9][a-z0-9-]{0,63}$`. It becomes a directory name, a
URL path segment and a build directory, so anything that would need escaping
anywhere is refused once, at upload.

## What is refused

| Refused | Why |
|---|---|
| An id that a built-in image already uses | An upload may add an application, not redefine what `mysql` installs |
| An archive with no `image.json` | There is nothing to add |
| An `image.json` with no `version` | Without one, no later upload could ever upgrade it |
| A member path that escapes the image directory | The extractor never writes outside `images/<id>/` |
| A symlink or special file | A package is regular files; a link is a way out of the directory |
| An archive over 64 MiB, unpacking over 192 MiB, or over 8192 files | Bounds what one request can make the server write |
| Anything `loadImage` rejects | A package that would not have worked is refused at upload, not at install |

Nothing reaches the live catalog until the archive has been unpacked into a
staging directory *and* loaded by the validator. A refused upload leaves the
previous version of that package exactly as it was.

## Replacing and removing

Uploading again with the same id **replaces** the package. What that refreshes
on every upload, because none of it lives in a container:

- the catalog entry — name, version, env fields, scopes;
- the `ui/` the browser loads;
- the source the `plugin/` is compiled from. Plugin processes for instances of
  that image are stopped, so the next call to one rebuilds and relaunches
  against the new source.

The container side is governed by **`version`**: if the package's version
differs from the one an installed copy recorded, its `install.sh` is run again
against that copy — immediately for copies that are running, and on next start
for copies that are stopped. Keep the version and no container is touched.
Per-copy results come back on the upload response. This is the whole of
[17 — Versions and upgrades](17-versions-and-upgrades.md); read it before
publishing a second version of anything.

Removing a package that is still installed would leave those copies pointing at
a catalog entry that no longer exists, so a plain `DELETE` refuses — and names
every scope and project the app is installed in, rather than saying only that
it is in use.

Asking to **uninstall first** does the whole job in one request: every copy is
uninstalled, in every scope, and then the package is deleted. That is the
button the UI offers, after listing the copies it is about to take down —
because uninstalling a `service` deletes its container along with its data.

A copy whose image can no longer be loaded — a package broken by a server
update — is dropped from the store rather than blocking the removal. There is
nothing left to describe how to tear it down, and refusing would leave an
operator with a package they can neither repair nor remove; whatever its
container still holds is theirs to clean up in LXD.

If a copy genuinely cannot be uninstalled — the container runtime is down — the
removal stops there, names that copy, and leaves the package in place, so a
retry does the whole job rather than half of it.

## When a stored package no longer loads

A package written against a different version of Remote may stop loading. It is
**skipped, not fatal**: the server starts, the built-in catalog is unaffected,
the package's files stay on disk, and the reason is reported next to it in
Settings → Applications so it can be re-uploaded or removed.

Refusing to boot over one administrator's file would turn a bad upload into an
outage. Deleting it silently would destroy the only copy.

## Trust

**An uploaded package is server code.** Its `install.sh` runs as root inside a
container; its `ui/` runs on the main origin with the SPA's privileges; its
`plugin/` is compiled and run as a child of the server process with the
server's privileges and the install's secrets.

That is why every route below is **admin-only** and why the ZIP is not a
sandbox. Uploading a package is the same act of trust as merging a directory
into `images/` — see [13 — Security model](13-security-model.md). Upload
packages you wrote or audited.

## HTTP

All admin-only. See [12 — HTTP API](12-http-api.md) for the rest of the surface.

### `GET /api/applications/packages`

Every stored package, newest upload first. A package that failed to load is
listed with the reason in `error`; `installs[]` says where each one is
currently installed, which is what a caller needs before deciding to remove it.

```json
[
  {
    "id": "s3disk",
    "name": "s3disk",
    "version": "0.1.0",
    "type": "tool",
    "filename": "s3disk.zip",
    "size": 150691,
    "sha256": "ad43ba10…",
    "uploadedAt": 1788630824,
    "uploadedBy": "admin@example.com",
    "installs": [
      { "instanceId": "a1b2", "name": "s3disk", "scope": "project",
        "projectId": "proj-7", "status": "running" }
    ]
  }
]
```

### `POST /api/applications/packages`

The archive, as `multipart/form-data` under the field **`package`**, or as a
raw request body. Returns `201` with the stored package, including `upgraded[]`
— the installed copies this upload re-provisioned because its `version` moved.

```bash
curl -X POST https://remote.example.com/api/applications/packages \
  -H 'Content-Type: application/zip' --data-binary @s3disk.zip
```

```json
{
  "id": "s3disk", "name": "s3disk", "version": "0.2.0", "type": "tool",
  "upgraded": [
    { "instanceId": "a1b2", "name": "s3disk", "scope": "project",
      "projectId": "proj-7", "fromVersion": "0.1.0", "toVersion": "0.2.0" }
  ]
}
```

| Status | Meaning |
|---|---|
| `201` | Added or replaced |
| `400` | No archive in the request, or it exceeded the transport cap |
| `409` | The id belongs to a built-in image |
| `422` | The archive is not a usable package; the body says why |
| `503` | This server stores no packages |

### `DELETE /api/applications/packages/{id}[?uninstall=true]`

Removes a stored package. `uninstall=true` uninstalls every installed copy
first; without it a package that is still installed is refused, so a plain
delete can never destroy a database's container as a side effect of tidying the
catalog.

```json
{ "ok": true, "uninstalled": [ { "instanceId": "a1b2", "name": "s3disk",
  "scope": "project", "projectId": "proj-7", "status": "running" } ] }
```

| Status | Meaning |
|---|---|
| `200` | Removed; `uninstalled[]` lists the copies it took down |
| `404` | No package with that id |
| `409` | Still installed (the body names where), a copy could not be uninstalled, or the id is a built-in image |

## Catalog entries say where they came from

`GET /api/applications/catalog` stamps every entry with `source`:

```json
{ "id": "s3disk", "name": "s3disk", "source": "uploaded", … }
```

`builtin` or `uploaded`. It is decided by the registry and overwrites whatever
`image.json` declared, so a package cannot describe itself as built in. The UI
uses it to badge uploaded applications and to offer removing them.
