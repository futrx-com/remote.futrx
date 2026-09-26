# Applications

This is the catalog of one-click installable apps ("Applications" tab). It sits
at the repository root, where it is discoverable without knowing the backend's
package layout, and is **embedded into the backend binary** (`//go:embed
applications` in [`../catalog.go`](../catalog.go), a module of its own because
`go:embed` reaches only downwards). A running server serves these applications plus
any an administrator has uploaded as a `.zip` — same shape, same validator,
stored outside the binary. See [Uploaded packages](../docs/dev/installable-applications/16-uploaded-packages.md).

**Two applications ship here: [`hello-remote/`](hello-remote/) and
[`code-server/`](code-server/).** Hello Remote is the worked example
— every supported capability composed into one installable package.
Real apps — MySQL, PostgreSQL, Redis, s3disk — live in their own
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
    backend/
      main.go        required host entry point and composition root
      api/           importable request-handling package
        api.go
      lifecycle/     importable host package for publisher/subscriber behavior
        events.go
      container/     Go source, compiled inside the target container
        main.go       one binary named after the application, or:
        cmd/my-agent/main.go
    ui/              the extension that calls it
```

Keep regular files at the application root limited to `README.md` and
`application.json`. Put custom provisioning files in `infra/`, the required
host executable at `backend/main.go`, importable host packages such as request
handling in `backend/api/` and event ownership in `backend/lifecycle/`,
container programs in `backend/container/`, and browser assets in `ui/`.
Remote generates one host Go module from the `backend/` root and its child host
packages, then builds `.`; `backend/container/` is excluded from that module.
For container Go,
`cmd/` itself is optional: a root main package in `backend/container/` builds
one binary named after the application ID. Use
`backend/container/cmd/<binary>/` when naming a binary explicitly or installing
more than one. If `cmd/*` exists, the root is not built as an executable.

## 📚 Full documentation: [`docs/dev/installable-applications/`](../docs/dev/installable-applications/)

Everything is documented in detail there — it lives in the repository's
documentation tree rather than in this directory, so the catalog holds applications
and nothing else. Start with
[its README](../docs/dev/installable-applications/README.md).

| I want to… | Read |
|---|---|
| Understand the system | [Overview](../docs/dev/installable-applications/01-overview.md) |
| Add a database or service | [Application capabilities](../docs/dev/installable-applications/03-application-capabilities.md), [Install scripts](../docs/dev/installable-applications/04-install-scripts.md) |
| Add a button or panel to the UI | [Tutorial](../docs/dev/installable-applications/07-tutorial-build-an-application.md) |
| Look up an `application.json` field | [application.json reference](../docs/dev/installable-applications/02-application-json.md) |
| Look up an extension API method | [Extension API](../docs/dev/installable-applications/06-extension-api.md) |
| Add a server-side feature in Go | [Application backends](../docs/dev/installable-applications/15-application-backends.md) |
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
   `version` is required — changing it is what reprovisions copies
   people already installed. See
   [Versions and upgrades](../docs/dev/installable-applications/17-versions-and-upgrades.md).
2. Add any capabilities the application needs. `service` declares a standardized
   systemd process; `infra/install.sh` performs only custom provisioning;
   `backend/container/` adds core-built container commands; `port.internal`
   exposes it; `hostTools[]` installs checksum-pinned host
   executables; `backend/main.go` adds server behavior; child host packages such
   as `backend/api/` and `backend/lifecycle/` keep cohesive concerns out of the
   composition package; `ui/` adds browser behavior; and `skills/` publishes
   project-agent workflows. These may be used independently or together where their
   validation rules allow it.
3. Optionally add `ui/` to contribute to the interface. The layout is the
   manifest: `scripts/main.js` is the entry, `style/*.css` are injected,
   `views/*.html` are loadable by name.
4. Optionally add a `backend/` executable for server-side work. Its `main.go`
   composes a value that implements `applications.Backend`; the application's
   `ui/` reaches it through `remote.backend.call(...)`. Keep request handling in
   `backend/api/` and each event publisher in `backend/lifecycle/`, then compose
   them at the root through `rpc.ServeWithRuntime`. See
   [Application backends](../docs/dev/installable-applications/15-application-backends.md).
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
`backend/internal/`. See [Security model](../docs/dev/installable-applications/13-security-model.md#backend-backends).

## The example app

[`hello-remote/`](hello-remote/) is the full capability example and is here to
be installed. It deliberately carries every composable capability:
custom infrastructure, a supervised service and port, health checking, host
tools, container-built commands, a host backend, UI, and a project skill. Its
manifest also fills every author-controlled model field. Installing it exercises
the catalog, every install-input behavior, connection metadata, both scopes,
every extension slot, and the complete application lifecycle — so if it works,
the feature works.

Install it globally *and* in a project to watch one application run as two processes
with two counters. Its [README](hello-remote/README.md) says what to look at
and why.

The larger developer fixtures described in [Fixtures](../docs/dev/installable-applications/10-fixtures.md) —
`ui-playground`, `ui-sandbox`, `backend-playground` — are not in this
repository.
