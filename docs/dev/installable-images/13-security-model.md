# 13 — Security model

## The trust boundary is the build, not the request

Extension code runs **on the main origin with the same privileges as the SPA**.
There is no sandbox, and none is implied. A `ui/` directory can:

- read and write the DOM of the whole application;
- call any Remote API endpoint as the signed-in user;
- read `localStorage`, and anything else same-origin JavaScript can reach.

What it cannot do is get there without a build. Assets are compiled into the
server binary by `//go:embed`, next to the SPA itself. There is no runtime
plugin directory, no upload endpoint, and no way to add an image to a running
server. Someone who can add a `ui/` directory can already ship arbitrary
frontend code by editing `frontend/src`.

**Therefore: treat a new or edited `ui/` in a pull request exactly as you treat
any other frontend change.** That is the control. Reviewing an image's
`install.sh` carefully while skimming its `ui/` gets the risk backwards — the
script runs in a disposable container, the extension runs in the user's
session.

## What is enforced, and where

| Property | Enforced by | Notes |
|---|---|---|
| Only catalog images exist | `//go:embed` | No runtime installation |
| A malformed image cannot ship | `registry.go:validate`, `registry_ui.go:loadImageUI` | Fails the build and the tests |
| Assets stay inside one image's `ui/` | `registry.go:cleanUIPath` | The only path out of the package |
| Only signed-in users fetch assets | `applications_handler.go` | Same gate as the catalog |
| Responses are not sniffable | `Content-Type` from extension + `nosniff` | Types are pinned, never guessed |
| Uninstalled extensions do not load | `Service.UIExtensions` | Installation is the gate |
| Project extensions stay in their project | `ExtensionRegistry.isInScope` | Frontend, per contribution, per render |
| A project you cannot see never reaches you | `ListVisible(email, isAdmin)` | Backend; the frontend never sees those entries |
| Global app management is admin-only | `requireAdmin` | Server-wide infrastructure |
| A project member cannot touch another project's app | `ensureProject` | Ownership re-checked per request |
| Secrets are not echoed to the UI | `View` / `envPublic` | Secret env values are redacted outside the credentials route |
| Only catalog source becomes a plugin | `//go:embed` + `registry_plugin.go` | No runtime plugin upload; `plugin/` must be `package main` and carry no module file |
| A plugin cannot act as its caller | `applications_backend_handler.go:forwardableHeaders` | `Cookie` and `Authorization` are withheld; the caller is supplied separately |
| A plugin's caller cannot be forged | `service/applications/backend.go:CallBackend` | `Request.Caller` is overwritten with the session's identity |
| A stopped app's plugin is unreachable | `Service.backendSpec` | `409` rather than a silent start |
| An admin-only plugin stays admin-only | `ImageBackend.Audience` + the service | Enforced before the process is reached |
| A plugin cannot write the session | `writeBackendResponse` | `Set-Cookie` is dropped; every response is `nosniff` |

## Backend plugins

A plugin is Go source from the catalog, compiled by the server and run as a
**child of the server process**. That is a bigger capability than a `ui/`
directory, and it is worth being explicit about it.

### What a plugin can do

- Everything the server process can: the filesystem, the network, `exec`.
  On a normal installation the server runs as root, so a plugin does too.
- Read the instance's resolved environment, **including the secrets its own
  install script generated** — a database plugin needs the password.
- Keep state, in memory and in the per-instance `DataDir` the host gives it.

There is no sandbox around it, and none is implied. A plugin is not
less-trusted code running under supervision; it is server code with a process
boundary, and the boundary exists for *robustness* — a panicking or hanging
plugin costs one call — not for containment.

### What stops it

The same thing that stops a malicious `ui/`: **the build**. Plugin source is
embedded with `//go:embed`, so it arrives only through a commit. There is no
upload endpoint and no runtime plugin directory.

**Therefore: review a new or edited `plugin/` exactly as you would review
`internal/`.** It is not "an app's config", it is server code that will run
with the server's privileges. Reviewing an image's `install.sh` carefully while
skimming its `plugin/` gets the risk backwards twice over: the script runs in a
disposable container, the extension runs in the user's session, and the plugin
runs on the host.

### What the platform does enforce

Between a browser and a plugin, the platform guarantees three things:

| Guarantee | Why it matters |
|---|---|
| `Request.Caller` is the session's identity, overwritten server-side | A plugin can authorize callers, because the browser cannot lie about who it is |
| `Cookie` and `Authorization` are never forwarded | A plugin is told who is asking without being handed the means to become them |
| `access: "admin"` is checked before the process is reached | An image can keep its plugin off non-admin sessions without writing the check itself |

Everything finer — which caller may do which thing — is the plugin's own job.
A plugin that ignores `Request.Caller` is as open as its `access` level, which
for the default `registered` means every signed-in user.

### Reviewing a plugin

- **Does it authorize?** If any route does something not every signed-in user
  should be able to do, it must check `request.Caller` itself.
- **What does it do with `Instance.Env`?** Those are real secrets. Using them
  is the point; returning them to a browser is a decision, and
  `backend-playground` shows the pattern — redact by caller.
- **Does it `exec` anything built from a request?** Command injection here is
  command injection as root.
- **Does it write outside `DataDir`?** `DataDir` is the storage the platform
  manages and cleans up. Anything else is unmanaged state on the host.
- **Does it reach the network?** Same exfiltration surface as a `ui/`, with
  more to exfiltrate and no browser between it and the internet.

### What a plugin does not get

- **A capability model.** There is no per-plugin permission set; there is the
  build boundary and `access`.
- **A resource limit.** No cgroup, no memory cap, no CPU share. A plugin that
  allocates without bound affects the host.
- **A supply chain.** Plugins may import only the standard library and this
  SDK, pinned to the versions the server itself was built with. That is a
  deliberate limitation rather than a solved problem: adding third-party
  modules to plugin builds would need an answer to provenance first.

If images ever become runtime-installable, none of this is adequate — see
below, and note that a runtime-installable *plugin* is a strictly harder
problem than a runtime-installable `ui/`.

## Path traversal

`Registry.UIAsset` is the only way `ui/` bytes leave the integration package,
and it is where traversal stops:

```go
func (r *Registry) UIAsset(imageID, assetPath string) ([]byte, bool)
```

It rejects an unknown image, an image with no `ui/`, an empty or absolute path,
anything containing a backslash, and any path that resolves outside the image's
own `ui/` after cleaning. Both `..` and percent-encoded `%2e%2e` are covered,
because the check runs on the decoded, cleaned path.

Verified requests, all `404`:

```
/api/applications/catalog/mysql/ui/../install.sh
/api/applications/catalog/mysql/ui/../../redis/install.sh
/api/applications/catalog/mysql/ui/%2e%2e/install.sh
/api/applications/catalog/mysql/ui/..%2finstall.sh
/api/applications/catalog/redis/ui/scripts/main.js      (redis ships no ui/)
```

Pinned by `registry_test.go:TestCleanUIPath` and `TestRegistryUIAsset`, and
re-checked at runtime by the `ui-playground` self-test.

## Serving HTML same-origin

Views are served as `text/html` on the application's own origin, so a view is
in principle a same-origin page. This is safe **only because of the build
boundary**: those bytes were compiled into the binary by whoever built the
server. It is not safe reasoning if images ever become runtime-installable —
see below.

## Two flavours of "safe"

The extension system is carefully **robust**: a throwing entry module, render,
predicate, or click handler is caught, and costs one extension its own UI.

That is not a security boundary. A malicious extension does not need to throw —
it can simply do the harmful thing correctly. Robustness protects against bugs;
the build boundary protects against malice.

## Runtime-installable images: uploaded packages

Images are also installable at runtime, as uploaded `.zip` packages — see
[16 — Uploaded packages](16-uploaded-packages.md). That does not weaken the
model above, because it does not widen who may add code. It moves the boundary
from *the build* to *the administrator*, and nowhere further:

- **Admin-only.** Every package route requires an administrator. The same
  account can already install a global application, change the base image, and
  run an install script as root in a container. Uploading a package is inside
  that authority, not beyond it.
- **Same validator.** An uploaded package loads through the same `loadImage`
  the embedded catalog does. There is no path a package can take that a
  built-in image cannot.
- **Same privileges, and no more.** The `ui/` runs on the main origin, the
  `plugin/` runs as a child of the server, the `install.sh` runs as root in a
  container. Exactly as they do for a built-in image.
- **No reserved id may be taken.** A package cannot claim the id of a built-in
  image, so it cannot redefine what an application the operator already trusts
  installs.
- **The extractor is the one new attack surface.** It refuses escaping paths,
  symlinks, special files, oversized members and compression bombs, and writes
  nothing executable. Everything lands under `$DATA_DIR/app-packages/images/<id>/`.

**Uploading a package is an act of trust identical to merging a directory into
`images/`.** Review one the same way — the checklist below applies unchanged,
and `plugin/` gets the [Backend plugins](#backend-plugins) checklist too.

What is still *not* there, and what it would take to let non-administrators
install extensions or to accept packages from an untrusted registry:

- **Isolation.** An iframe on a separate origin with a `postMessage` bridge,
  rather than direct DOM and `fetch` access. Slots would post render intents
  instead of receiving elements.
- **A capability model.** Explicit, reviewable permissions per extension rather
  than the ambient authority of the signed-in session.
- **CSP.** A policy that stops an extension reaching arbitrary third-party
  origins.
- **Signing and provenance.** Some answer to "who wrote this and did it change".
  The store records who uploaded a package and its SHA-256; it verifies neither
  against anything.

Do not widen the audience for uploads without them.

## Reviewing an extension

A checklist for reviewing a `ui/` directory:

- **`innerHTML` with interpolated data.** Views are static markup, but anything
  built from API responses or user input needs escaping. `ui-playground` has an
  `escapeHtml` helper for exactly this.
- **Where does it send data?** `fetch` to anything not same-origin is
  exfiltration surface. There is no CSP stopping it.
- **What does it read?** Credentials endpoints return real passwords. An
  extension reading them is legitimate (MySQL's does) — but it should display
  them, not transmit them.
- **Does it touch the app's DOM outside its host elements?** Unsupported, will
  break, and may be doing something it should not.
- **Does the install scope match the intent?** A plugin meant for one project
  should not be documented as a global install.
- **Does it ship a `plugin/`?** Then review that too, against the checklist in
  [Backend plugins](#backend-plugins) above — it is server code, not frontend
  code.

## Related

- [12 — HTTP API](12-http-api.md) — the authorization of each endpoint.
- [08 — Scoping and visibility](08-scoping-and-visibility.md) — who sees what.
- The platform-wide [threat model](../../threat-model.md) — the
  boundaries this sits inside.
