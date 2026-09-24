# 13 — Security model

## The trust boundary is package admission, not the request

Extension code runs **on the main origin with the same privileges as the SPA**.
There is no sandbox, and none is implied. A `ui/` directory can:

- read and write the DOM of the whole application;
- call any Remote API endpoint as the signed-in user;
- read `localStorage`, and anything else same-origin JavaScript can reach.

What it cannot do is get there without trusted package admission. Built-in
assets are compiled into the server binary by `//go:embed`, next to the SPA
itself. Uploaded application ZIPs pass the same validator and require an
administrator, an account that is already allowed to install server-wide
infrastructure and admit application code. Someone who can add a built-in
`ui/` directory can already ship arbitrary frontend code by editing
`frontend/src`; an administrator uploading a package is making the same trust
decision at runtime.

**Therefore: treat a built-in `ui/` change exactly as any other frontend
change, and review an uploaded package before admitting it.** That is the
control. Reviewing an application's
`infra/install.sh` carefully while skimming its `ui/` gets the risk backwards — the
script runs in a disposable container, the extension runs in the user's
session.

## What is enforced, and where

| Property | Enforced by | Notes |
|---|---|---|
| Only validated catalog applications exist | embedded catalog or admin-only package upload through `Registry` | No loose runtime source directory or unvalidated package path |
| A malformed application cannot enter the catalog | `registry.go:validate`, `registry_ui.go:loadApplicationUI` | Fails built-in startup/tests or the package upload |
| Only trusted product code selects automatic installs | `builtInDefaultApplicationIDs` + `Service.ReconcileDefaultApplications` | Each ID must be built in and globally installable; packages cannot declare themselves a default |
| Assets stay inside one application's `ui/` | `registry.go:cleanUIPath` | The only path out of the package |
| Only signed-in users fetch assets | `applications_handler.go` | Same gate as the catalog |
| Responses are not sniffable | `Content-Type` from extension + `nosniff` | Types are pinned, never guessed |
| Uninstalled extensions do not load | `Service.UIExtensions` | Installation is the gate |
| Project extensions stay in their project | `ExtensionRegistry.isInScope` | Frontend, per contribution, per render |
| A project you cannot see never reaches you | `ListVisible(email, isAdmin)` | Backend; the frontend never sees those entries |
| Global app management is admin-only | `requireAdmin` | Server-wide infrastructure |
| A project member cannot touch another project's app | `ensureProject` | Ownership re-checked per request |
| Secrets are not echoed to the UI | `View` / `envPublic` | Secret env values are redacted outside the credentials route |
| Only validated catalog source becomes a backend | `registry_backend.go` for built-in and uploaded packages | the `backend/` root must be `package main`; child host packages share Remote's generated module; host `go.mod`, `go.sum`, `go.work`, and `go.work.sum` files are refused; `backend/container/` is excluded |
| A backend cannot act as its caller | `applications_backend_handler.go:forwardableHeaders` | `Cookie` and `Authorization` are withheld; the caller is supplied separately |
| A backend's caller cannot be forged | `service/applications/backend.go:CallBackend` | `Request.Caller` is overwritten with the session's identity |
| A stopped app's backend is unreachable | `Service.backendSpec` | `409` rather than a silent start |
| An admin-only backend stays admin-only | `ApplicationBackend.Audience` + the service | Enforced before the process is reached |
| A backend cannot write the session | `writeBackendResponse` | `Set-Cookie` is dropped; every response is `nosniff` |
| A streamed backend response cannot dictate HTTP framing | `writeBackendResponse` + `http.ServeContent` | core drops supplied `Content-Length`, owns range/status handling, and closes content on cancellation |
| One streamed read has bounded cross-process memory | `pkg/applications/rpc/stream.go` | absolute reads are capped at 256 KiB; the application passes an open reader, never a path for core to reopen |

## Application backends

A backend is Go source from the catalog, compiled by the server and run as a
**child of the server process**. That is a bigger capability than a `ui/`
directory, and it is worth being explicit about it.

### What a backend can do

- Everything the server process can: the filesystem, the network, `exec`.
  On a normal installation the server runs as root, so a backend does too.
- Read the instance's resolved environment, **including the secrets its own
  install script generated** — a database backend needs the password.
- Keep state, in memory and in the per-instance `DataDir` the host gives it.

There is no sandbox around it, and none is implied. A backend is not
less-trusted code running under supervision; it is server code with a process
boundary, and the boundary exists for *robustness* — a panicking or hanging
backend costs one call — not for containment.

A streamed response does not widen that privilege. The backend opens the
content itself and transfers ownership of an `io.ReadSeekCloser`; core receives
only bounded reads over the process broker, not a filesystem path. This keeps
path validation and file authority in the application that owns the feature.
Core closes the reader when the HTTP request finishes or is cancelled, and a
stream that arrives after the backend-call deadline is closed without being
exposed to the caller.

### What admits it

The same authority boundary as a `ui/`: **the build or an administrator's
package upload**. Built-in backend source arrives through a commit and
`//go:embed`; uploaded backend source passes the same validator before it joins
the live catalog. There is no loose runtime backend directory.

**Therefore: review a new or edited `backend/` exactly as you would review
`internal/`.** It is not "an app's config", it is server code that will run
with the server's privileges. Reviewing an application's `infra/install.sh` carefully while
skimming its `backend/` gets the risk backwards twice over: the script runs in a
disposable container, the extension runs in the user's session, and the backend
runs on the host.

### What the platform does enforce

Between a browser and a backend, the platform guarantees four things:

| Guarantee | Why it matters |
|---|---|
| `Request.Caller` is the session's identity, overwritten server-side | A backend can authorize callers, because the browser cannot lie about who it is |
| `Cookie` and `Authorization` are never forwarded | A backend is told who is asking without being handed the means to become them |
| `access: "admin"` is checked before the process is reached | An application can keep its backend off non-admin sessions without writing the check itself |
| `Request.Context.Chat` exists only after core authorizes that chat; project installs must match its project | A backend can use the verified workspace root without trusting a browser-supplied host path |

Everything finer — which caller may do which thing — is the backend's own job.
A backend that ignores `Request.Caller` is as open as its `access` level, which
for the default `registered` means every signed-in user.

Chat context is authorization metadata, not a filesystem sandbox. The backend
still runs with server privileges and could access other host paths on its own.
The value prevents a caller from selecting another chat's workspace through
the supported API; it does not contain trusted backend code.

### Reviewing a backend

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

### What a backend does not get

- **A capability model.** There is no per-backend permission set; there is the
  package-admission boundary and `access`. Event declarations constrain which
  publisher names and versions a backend may emit, but do not sandbox its OS,
  filesystem, network, or process access.
- **A resource limit.** No cgroup, no memory cap, no CPU share. A backend that
  allocates without bound affects the host.
- **A supply chain.** Backends may import only the standard library and this
  SDK, pinned to the versions the server itself was built with. That is a
  deliberate limitation rather than a solved problem: adding third-party
  modules to backend builds would need an answer to provenance first.

If package admission is ever widened beyond administrators, none of this is
adequate — see below. Admitting a backend is strictly harder than admitting a
UI because it runs with the server's privileges.

## Backend event security

Manifest event declarations constrain routing; they do not make an application
backend untrusted code safe. The backend still has all server-process authority
described above.

| Guarantee | Boundary |
|---|---|
| A backend may publish only a manifest-declared local publisher, event, and version | Host validates every `Publication` before dispatch |
| A backend cannot forge its event identity | Host stamps application ID, instance ID, scope, project ID, and canonical publisher |
| One package cannot claim another package's namespace | Application publishers are canonicalized as `applications.<application-id>.<local-publisher>`; `remote` and `applications` are reserved local prefixes |
| A project-origin event cannot escape its project | Only project-scoped subscribers in the same project are eligible; global subscriber instances do not receive it |
| A malformed or oversized payload is rejected | Payload must be a non-null JSON object of at most 64 KiB |

There are three important non-guarantees:

- **`OnEvent` is machine-to-machine.** It has no signed-in user and no
  `Request.Caller`. The manifest backend `access` setting authorizes browser
  calls only; it does not restrict event delivery. A handler must base its
  decision on the declared publisher, host-stamped source, scope, and validated
  payload rather than inventing a user identity.
- **Custom payloads are not redacted.** Remote validates their shape and size
  but does not understand their fields, remove passwords, or filter values for
  individual recipients. A publisher must never include a secret it does not
  intend every scope-eligible subscriber to receive. Subscribers must treat
  payload data as untrusted even though source identity is trusted.
- **Delivery is not an authorization acknowledgement.** Publish acceptance
  does not wait for consumers. Handler failures and timeouts cannot fail the
  publication or prevent later recipients; a timeout does terminate that
  subscriber process and may interrupt its concurrent calls. Events are not
  persisted or replayed.

Review an event-capable backend for both sides: what data it publishes and
whether every possible scope-eligible recipient may see it; then how it
validates publisher, name, version, and payload before acting. Event behavior
belongs in the importable `backend/lifecycle/` owner, but it runs with the
`backend/` executable in the same unsandboxed process and has the same authority. The
full routing contract is
[18 — Backend event lifecycle](18-application-events.md).

## Path traversal

`Registry.UIAsset` is the only way `ui/` bytes leave the integration package,
and it is where traversal stops:

```go
func (r *Registry) UIAsset(applicationID, assetPath string) ([]byte, bool)
```

It rejects an unknown application, an application with no `ui/`, an empty or absolute path,
anything containing a backslash, and any path that resolves outside the application's
own `ui/` after cleaning. Both `..` and percent-encoded `%2e%2e` are covered,
because the check runs on the decoded, cleaned path.

Verified requests, all `404`:

```
/api/applications/catalog/mysql/ui/../infra/install.sh
/api/applications/catalog/mysql/ui/../../redis/infra/install.sh
/api/applications/catalog/mysql/ui/%2e%2e/infra/install.sh
/api/applications/catalog/mysql/ui/..%2finfra/install.sh
/api/applications/catalog/redis/ui/scripts/main.js      (redis ships no ui/)
```

Pinned by `registry_test.go:TestCleanUIPath` and `TestRegistryUIAsset`, and
re-checked at runtime by the `ui-playground` self-test.

## Serving HTML same-origin

Views are served as `text/html` on the application's own origin, so a view is
in principle a same-origin page. This is acceptable only because admission is
trusted: those bytes were either compiled into the binary by its builder or
accepted by an administrator through the package validator. It would not be a
safe model for uploads from untrusted users.

## Two flavours of "safe"

The extension system is carefully **robust**: a throwing entry module, render,
predicate, or click handler is caught, and costs one extension its own UI.

That is not a security boundary. A malicious extension does not need to throw —
it can simply do the harmful thing correctly. Robustness protects against bugs;
the builder/administrator admission boundary protects against malice.

## Runtime-installable applications: uploaded packages

Applications are also installable at runtime, as uploaded `.zip` packages — see
[16 — Uploaded packages](16-uploaded-packages.md). That does not weaken the
model above, because it does not widen who may add code. It moves the boundary
from *the build* to *the administrator*, and nowhere further:

- **Admin-only.** Every package route requires an administrator. The same
  account can already install a global application, change the base image, and
  run an install script as root in a container. Uploading a package is inside
  that authority, not beyond it.
- **Same validator.** An uploaded package loads through the same `loadApplication`
  the embedded catalog does. There is no path a package can take that a
  built-in application cannot.
- **Same privileges, and no more.** The `ui/` runs on the main origin, the
  `backend/` runs as a child of the server, `infra/install.sh` runs as root in a
  container. Exactly as they do for a built-in application.
- **No reserved id may be taken.** A package cannot claim the id of a built-in
  application, so it cannot redefine what an application the operator already trusts
  installs.
- **No package can become a default.** Automatic installation is a core-owned
  list and reconciliation checks catalog provenance again before acting. The
  successful one-time marker is kept outside instance records, so uninstalling
  a default remains an administrator choice rather than a temporary state.
- **The extractor is the one new attack surface.** It refuses escaping paths,
  symlinks, special files, oversized members and compression bombs, and writes
  nothing executable. Everything lands under `$DATA_DIR/app-packages/applications/<id>/`.

**Uploading a package is an act of trust identical to merging a directory into
`applications/`.** Review one the same way — the checklist below applies unchanged,
and `backend/` gets the [Application backends](#application-backends) checklist too.

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
- **Does the install scope match the intent?** A backend meant for one project
  should not be documented as a global install.
- **Does it ship a `backend/`?** Then review that too, against the checklist in
  [Application backends](#application-backends) above — it is server code, not frontend
  code.

## Related

- [12 — HTTP API](12-http-api.md) — the authorization of each endpoint.
- [08 — Scoping and visibility](08-scoping-and-visibility.md) — who sees what.
- The platform-wide [threat model](../../threat-model.md) — the
  boundaries this sits inside.
