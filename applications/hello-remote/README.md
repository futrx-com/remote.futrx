# Hello Remote

The catalog's reference example. It combines broad application capabilities:

- **`hostTools[]`** — a checksum-pinned, compressed `restic` executable installed on the Remote host.
- **Manifest infrastructure fields** — a declarative systemd service, TCP
  proxy, health check, and a greeting that changes application behavior.
- **`infra/install.sh`** — idempotent custom provisioning that creates a
  dedicated service account and persistent application state directory.
- **`backend/container/`** — Go source Remote copies into and builds inside LXD;
  one command inspects the container and another serves HTTP.
- **`backend/main.go`** — the required Go executable and composition root the
  server compiles and runs as a child process, reachable at
  `/api/applications/<instance>/backend/<path>`.
- **`backend/api/`** — the importable request-handling package composed by the
  root executable.
- **`backend/lifecycle/`** — the importable child package that owns event
  publication inside that same process.
- **Application events** — separate `greetings.greeted` and
  `inspections.container-inspected` publishers. Hello Remote does not
  subscribe to core or application events.
- **`ui/`** — assets the SPA loads for users who installed the application, which
  call that backend through `remote.backend.call(...)`.
- **`skills/`** — an agent skill published into project workspaces.

Its container source builds `hello-remote-info` and `hello-remote-service`. The
manifest supervises the latter as `hello-remote.service` and exposes its
internal port through a loopback-only host proxy. A project
installation uses the project's existing LXD container. A global installation
uses a dedicated application container. Both are ordinary sibling containers on
the host—there is no LXD inside LXD. The host backend invokes the inspection
command with `lxc exec` and reaches the service through the allocated proxy.
Raw LXD configuration and environment variables are deliberately not returned.

Hello Remote's custom install script demonstrates the narrow work that belongs
under `infra/`: it idempotently creates a dedicated `hello-remote` system
account, provisions `/var/lib/hello-remote`, and atomically records the
installed application version there. The service reads that version and the UI
displays it, making the custom provisioning observable. On an upgrade, the
script converges the identity and directory, updates only its metadata file,
and preserves other application state.

Remote still owns the standardized work. It builds the container Go programs,
writes the root-only encoded environment, creates the hardened systemd unit,
declares its daemon to the idle-workspace probe, restarts it idempotently,
waits for the manifest health check, installs the host tool, and owns the proxy
and lifecycle. The install script neither writes a unit nor invokes `systemctl`.

The backend runs on the Remote host, not inside LXD. The generic container
capability supplies `ContainerName`, without application-specific packaging.

## Installing it

| Scope | Where |
|---|---|
| Global | **Settings → Applications** |
| Project | **Project → Applications** |

The install dialog has one application-specific input: a defaulted greeting.
It drives both the container service and the host backend's `Instance.Env`, so
changing it produces an observable result. Hello Remote deliberately declares
no connection user, password, or database because it implements none of those
resources.

The preferred host port is `4780`, bound to `127.0.0.1`; Remote automatically
chooses another host port if it is occupied. The internal service remains on
`4780`. The install also downloads the declared `restic` binary for the host's
architecture, verifies its SHA-256 digest before decompression, runs `restic version`, and publishes it
under Remote's own data directory.

Install it at both scopes to compare a dedicated global application container
with an existing project container. Each installation has a separate backend
process, `DataDir`, and counter.

**A server that runs an application backend needs a Go toolchain**, because backend
source is compiled on the host. Without one, the install reports that on the
instance instead of failing the server. See
[14 — Troubleshooting](../../docs/dev/installable-applications/14-troubleshooting.md).

## What it does

Hello Remote deliberately contributes controls throughout the product so this
directory is a visual catalog of the frontend extension API:

| Where | What |
|---|---|
| Sidebar header and search | Compact icon buttons through `ui.addIconButton` |
| Every project row | A context-aware icon with that project's id and name |
| Chat header and composer | Icons that receive the active project, chat, and working directory context |
| This application's card | Labeled `ui.addButton` controls, scoped with `when` |
| A panel below the applications list | Shows the greeting, counter, supervised service, port mapping, and live container facts |
| Project settings | A custom panel mounted through `ui.register` with cleanup |

Every API icon opens the same capability explorer. It displays `apiVersion`,
application metadata, install visibility, slot context, backend availability,
and observed `upload.completed` events. Its controls exercise
`backend.call` (including method, JSON body, query, headers, and cancellation),
`backend.fetch`, `backend.describe`, `backend.url`,
`views.load`, `views.url`, `assets.url`, `remote.log`, and popup cleanup.
The explorer can also arm a one-shot `event.claim`: the next upload is claimed
synchronously but resolves to its original path, demonstrating the contract
without moving or deleting the user's attachment.

The greeting comes back as `"<greeting>, <your email>."`. The email is proof
of something worth seeing: the browser never sent it. The server stamps the
signed-in caller onto every forwarded request and withholds the cookies that
authenticated it, so a backend can tell who is asking and cannot act as them.

The counter is proof of the other half. The backend process is killed on stop,
on uninstall, and on server restart, and is started again lazily by the next
call — so a count that survives is a count that reached `DataDir`. Restart the
server, open the panel, and the number is still there.

Each successful counter increment publishes the manifest-declared
`greetings.greeted` event, and each successful container inspection publishes
`inspections.container-inspected`. Both carry versioned JSON payloads.
`backend/lifecycle/` owns the two typed publisher interfaces and their concrete
emitters while Remote constructs and binds the event runtime from the manifest.
The API depends on those interfaces; it does not implement or initialize a
publisher. The manifest deliberately
declares no subscriptions: Remote core publishes its application lifecycle
events automatically, independently of whether Hello Remote consumes them.
Emission happens outside the
visit-counter lock and a publication failure becomes a response warning; it
never rolls back a greeting.

The service section crosses the allocated host proxy and reports the systemd
unit, build version, custom-provisioning version, port mapping, and greeting
response. The container section reports
its LXD name, hostname, operating system, kernel, architecture, CPU count, total
memory, and uptime. Each has an independent **Refresh** action.

## Reading it

| File | Shows |
|---|---|
| `application.json` | The application identity, both scopes, base image, real greeting input, port, service, install path, health check, host tool, two publishers, explicit UI mapping, and backend policy. |
| `backend/main.go` | The required executable and composition root. It receives core-owned runtime capabilities and wires the API to the lifecycle publishers. |
| `backend/api/api.go` | The host backend contract (`Describe`, `Init`, `Handle`) and request router. |
| `backend/api/greeting.go`, `backend/api/visits.go` | Greeting and echo routes, the per-instance persistent counter, and the request-side business trigger for the greeting event. |
| `backend/api/container.go`, `backend/api/service.go` | Bounded host calls into the installed inspection command and the proxied HTTP service. |
| `backend/lifecycle/greetings.go` | The typed `greetings.greeted` identity and payload. It emits through `Runtime.Events`; publisher registration and validation remain core-owned. |
| `backend/lifecycle/inspections.go` | The independent `inspections.container-inspected` publisher contract and payload. |
| `backend/container/cmd/hello-remote-info/main.go` | The container program. Only `package main` and `func main()` are required. |
| `backend/container/cmd/hello-remote-service/` | A supervised HTTP service with separate command dispatch, configuration decoding, serving, and health-probe owners. |
| `backend/container/internal/containerinfo/` | Container-only inspection code and tests. Remote packages and builds it without backend-owned shell. |
| `application.json.service` | Command, environment mappings, process identity, restart policy, and systemd hardening. |
| `infra/install.sh` | Idempotent service-account and persistent-state provisioning using platform-supplied install metadata. |
| `skills/hello-remote-inspector/SKILL.md` | A project-scoped agent workflow that verifies the service and its runtime metadata. |
| `ui/scripts/main.js` | The entry module: activates the showcase, card action, applications panel, and cleanup. |
| `ui/scripts/containerPanel.js`, `ui/scripts/servicePanel.js`, `ui/scripts/inspectionRefresh.js` | Container and service presentation with one disposal-safe refresh lifecycle. |
| `ui/scripts/showcase.js`, `ui/scripts/showcaseExplorer.js` | Slot registration and the live explorer for the complete frontend API. |
| `ui/scripts/uploadTracker.js` | Cohesive upload observation and one-shot pass-through claim state. |
| `ui/views/panel.html`, `ui/style/hello.css` | The two conventions — views loaded by name, CSS written against the platform's theme tokens. |

Full documentation is in [`docs/dev/installable-applications/`](../../docs/dev/installable-applications/); the tutorial that builds an
application from nothing is
[07 — Tutorial](../../docs/dev/installable-applications/07-tutorial-build-a-backend.md).

## Editing it

The `ui/` and `backend/` trees are embedded in the
binary, so changes need a backend rebuild — `npm run dev` will not pick them up.
A `backend/` edit is recompiled by the server on the next start because the
build fingerprint changed. Container-source changes also change its generated
build identity, so installed copies are reprovisioned without regenerating an archive.
