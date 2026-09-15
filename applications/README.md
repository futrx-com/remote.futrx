# Applications

This is the catalog of one-click installable apps ("Applications" tab). It sits
at the repository root, where it is discoverable without knowing the backend's
package layout, and is **embedded into the backend binary** (`//go:embed
applications` in [`../catalog.go`](../catalog.go), a module of its own because
`go:embed` reaches only downwards). A running server serves these applications plus
any an administrator has uploaded as a `.zip` — same shape, same validator,
stored outside the binary. See [Uploaded packages](../docs/dev/installable-applications/16-uploaded-packages.md).

**One application ships here: [`hello-remote/`](hello-remote/)**, the worked example
— a Go backend and the browser UI that calls it, installing nothing in any
container. Real apps — MySQL, PostgreSQL, Redis, s3disk — live in their own
repositories and reach a server as uploaded packages, so the catalog format can
change here without a database application riding along in the same review.
Everything below is the format they are all written against, and dropping a
directory in here is still all it takes to build one in.

An application is one directory. It can install software into a container, contribute
to the Remote interface from the browser, add a Go backend that runs on the
server, or any combination:

```
applications/
  postgresql/
    README.md        application documentation
    application.json metadata: name, description, port, env, systemd service
    infra/
      install.sh     idempotent installer, run as root inside the container
  object-mount/
    README.md
    application.json installs into the project container, no port
    infra/install.sh
  ui-playground/
    README.md
    application.json
    ui/              browser extension: buttons, panels, popups
      views/*.html
      style/*.css
      scripts/main.js
  backend-playground/
    README.md
    application.json
    backend/         Go source, compiled by the server and run as a process
      main.go
    ui/              the extension that calls it
```

Keep regular files at the application root limited to `README.md` and
`application.json`. Put provisioning files in `infra/`, server code in
`backend/`, and browser code and assets in `ui/`.

## 📚 Full documentation: [`docs/dev/installable-applications/`](../docs/dev/installable-applications/)

Everything is documented in detail there — it lives in the repository's
documentation tree rather than in this directory, so the catalog holds applications
and nothing else. Start with
[its README](../docs/dev/installable-applications/README.md).

| I want to… | Read |
|---|---|
| Understand the system | [Overview](../docs/dev/installable-applications/01-overview.md) |
| Add a database or service | [Application capabilities](../docs/dev/installable-applications/03-application-capabilities.md), [Install scripts](../docs/dev/installable-applications/04-install-scripts.md) |
| Add a button or panel to the UI | [Tutorial](../docs/dev/installable-applications/07-tutorial-build-a-plugin.md) |
| Look up an `application.json` field | [application.json reference](../docs/dev/installable-applications/02-application-json.md) |
| Look up an extension API method | [Extension API](../docs/dev/installable-applications/06-extension-api.md) |
| Add a server-side feature in Go | [Backend plugins](../docs/dev/installable-applications/15-backend-plugins.md) |
| Know where I can render | [Slots](../docs/dev/installable-applications/05-slots.md) |
| Know who sees my extension | [Scoping and visibility](../docs/dev/installable-applications/08-scoping-and-visibility.md) |
| Match the app's look | [Styling and icons](../docs/dev/installable-applications/09-styling-and-icons.md) |
| Test it | [Fixtures](../docs/dev/installable-applications/10-fixtures.md), [Testing](../docs/dev/installable-applications/11-testing.md) |
| Call the endpoints | [HTTP API](../docs/dev/installable-applications/12-http-api.md) |
| Understand the trust model | [Security model](../docs/dev/installable-applications/13-security-model.md) |
| Fix something broken | [Troubleshooting](../docs/dev/installable-applications/14-troubleshooting.md) |
| Add an app to a running server, without a release | [Uploaded packages](../docs/dev/installable-applications/16-uploaded-packages.md) |
| Ship a new version to people who already installed it | [Versions and upgrades](../docs/dev/installable-applications/17-versions-and-upgrades.md) |

## Adding an app, in short

1. Create `applications/<id>/application.json`. `id` must equal the directory name, and
   `version` is required — changing it is what re-runs `infra/install.sh` on copies
   people already installed. See
   [Versions and upgrades](../docs/dev/installable-applications/17-versions-and-upgrades.md).
2. Add any capabilities the application needs. `infra/install.sh` provisions a
   container; `port.internal` exposes it; `backend/` adds server behavior; and
   `ui/` adds browser behavior. These may be used independently or together.
3. Optionally add `ui/` to contribute to the interface. The layout is the
   manifest: `scripts/main.js` is the entry, `style/*.css` are injected,
   `views/*.html` are loadable by name.
4. Optionally add `backend/` for server-side work. `main.go` implements
   `appplugin.Backend`; the application's `ui/` reaches it through
   `remote.backend.call(...)`. See
   [Backend plugins](../docs/dev/installable-applications/15-backend-plugins.md).
5. Rebuild the backend. `NewRegistry()` validates every entry at startup, so a
   malformed application fails the build and the tests rather than 404ing in a
   browser.

No other code changes are required — the app appears in the catalog
automatically for both scopes.

## Adding an app without a release

An administrator can upload the same directory as a `.zip` from **Settings →
Applications**. It is stored in the server's state directory, loads through
this exact validator, and becomes an ordinary catalog entry — and it survives
updates, because updating replaces the binary and never touches that
directory. See [Uploaded packages](../docs/dev/installable-applications/16-uploaded-packages.md).

## Three things that surprise people

**Being in the catalog grants nothing.** An extension loads only after a user
installs the application — globally, or in a project they belong to — and only while
that instance is running. Stopping an app turns its UI off. See
[Scoping and visibility](../docs/dev/installable-applications/08-scoping-and-visibility.md).

**Extension code is frontend code.** It runs on the main origin with the SPA's
privileges; the trust boundary is the build, not the request. A new `ui/` in a
pull request deserves the same review as any change under `frontend/src`. See
[Security model](../docs/dev/installable-applications/13-security-model.md).

**Backend code is server code.** A `backend/` directory is compiled and run as a
child of the server process, with the server's privileges, and is handed the
install's secrets. It deserves the same review as any change under
`backend/internal/`. See [Security model](../docs/dev/installable-applications/13-security-model.md#backend-plugins).

## The example app

[`hello-remote/`](hello-remote/) is the one application this repository ships, and it
is here to be installed. It has `backend/` and `ui/` capabilities but no
`infra/install.sh`, so it needs no LXD, port, or proxy device and works on a laptop. Installing it exercises the
catalog, the install dialog's `env[]` field, both extension slots it draws in,
and a real backend process — so if it works, the feature works.

Install it globally *and* in a project to watch one application run as two processes
with two counters. Its [README](hello-remote/README.md) says what to look at
and why.

The larger developer fixtures described in [Fixtures](../docs/dev/installable-applications/10-fixtures.md) —
`ui-playground`, `ui-sandbox`, `backend-playground` — are not in this
repository.
