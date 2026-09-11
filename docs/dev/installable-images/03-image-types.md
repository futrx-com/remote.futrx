# 03 — Image types

`type` in `image.json` decides what installing an image actually *does*, and in
particular whether it touches a container at all.

| type | Installing it | Requires |
|---|---|---|
| `service` (default) | Runs software on a port under systemd | `port.internal`, an install script |
| `tool` | Provisions software into the project's container and exposes nothing | an install script, project scope only |
| `ui` | Nothing in any container — turns on the image's browser extension | a `ui/` directory |
| `backend` | Nothing in any container — compiles and runs the image's Go plugin on the host | a `plugin/` directory |

The Go type is `Kind` in
[`service/applications/model.go`](../../../backend/internal/service/applications/model.go).
Omitting `type` means `service`, so every image written before this field
existed keeps working unchanged.

`type` says what the image's *payload* is, not what it may carry. A `ui/`
directory works on any type, and so does `plugin/` — a `service` image can
provision MySQL, add a "Connect" button, and run a Go plugin that answers the
button's queries. `type` only decides whether installing it has to reach a
container.

## `service`

A service image provisions real software. Where it runs depends on the
**scope** the user installs it at.

### Global scope — its own LXD container

There is no project container to live in, so the app gets a dedicated one named
`futrx-app-<instanceID>`, launched from `base` (default `ubuntu:24.04`).

```
lxc info   futrx-app-abc123                       does it exist?
lxc launch ubuntu:24.04 futrx-app-abc123          create it
lxc exec   futrx-app-abc123 -- sh -c "ip -4 …"    wait for network
lxc exec   futrx-app-abc123 --env APP_INTERNAL_PORT=3306 -- bash -s
lxc config device add futrx-app-abc123 app-abc123 proxy \
    listen=tcp:127.0.0.1:3307 connect=tcp:127.0.0.1:3306 bind=host
```

### Project scope — the project's existing container

The project already has a container and the user is working in it. Installing
there must **not** create a second one: it would double the memory cost and put
the app off the project's filesystem.

```
lxc exec   my-project --env APP_INTERNAL_PORT=3306 -- bash -s
lxc config device add my-project app-abc123 proxy \
    listen=tcp:127.0.0.1:3307 connect=tcp:127.0.0.1:3306 bind=host
```

No `launch`, no `init`, no `copy`, no `create`. This is asserted by
`installer_test.go:TestInstallProjectScopeUsesTheProjectContainer`, which
checks for the *absence* of those commands.

### Lifecycle differences

The scope split carries through the whole lifecycle, not just install:

| Action | Global scope | Project scope |
|---|---|---|
| Install | launch the dedicated container, run the script, add the proxy | run the script in the project container, add the proxy |
| Start | start the container, `systemctl start <service>`, re-add the proxy | `systemctl start <service>`, re-add the proxy |
| Stop | remove the proxy, `lxc stop --force` the container | remove the proxy, `systemctl stop <service>` only |
| Uninstall | `lxc delete --force` the container | remove the proxy, `systemctl disable --now <service>`; packages and data stay |

Stopping a project app must never stop the container someone is working in, and
uninstalling one must never delete their project. Both are pinned by tests.

Start is not an install. It repairs what the app needs outside its container's
filesystem — its host tools, its skills, its proxy device — and starts its
service, but it does not re-run the install script: provisioning an app is
minutes of `apt-get` that switching it on has no reason to pay for. The one
exception is a copy whose image has moved on, which the service routes to
Install instead; see
[17 — Versions and upgrades](17-versions-and-upgrades.md).

### Reaching a service

Every installed service is reachable on the host through an LXD **proxy
device**:

```
listen  = <protocol>:<bindAddress>:<externalPort>   on the host
connect = <protocol>:127.0.0.1:<internalPort>       inside the container
```

A project app is additionally reachable from inside the project's own container
on the LXD bridge at `<slug>.lxd:<internalPort>`.

## `tool`

A tool image provisions software into a container exactly as a service does —
same install script, same systemd unit, same idempotency rules — but exposes
nothing. No port is allocated, no proxy device is created, and nothing outside
the container can reach it.

That is the point. The value of a tool is that it is *present in the container
someone is working in*: a CLI on the `PATH`, a mounted filesystem, an agent. A
port would be a lie, and modelling one as a `service` would allocate a host port
for something that is not listening.

A mount tool is the worked example: it installs a FUSE binary and mounts a
bucket at a path inside the project, under a systemd unit. Nothing listens.

### Project scope only

A tool declares `"scopes": ["project"]`. A global install would launch a
dedicated container, provision the tool into it, and hand it to nobody — the
container exists for the app, and there is no workspace in it to improve.
`registry.go:validate` rejects a tool that claims global scope.

### What install / start / stop / uninstall mean

Identical to a project-scope service, minus the proxy device:

| Action | Effect |
|---|---|
| Install | Run the install script in the project's container. No port is allocated. |
| Start | `systemctl start <service>`. The install script is not re-run. |
| Stop | `systemctl stop <service>`. The project container keeps running. |
| Uninstall | `systemctl disable --now <service>`. Installed packages and data stay. |
| Set port | Rejected with `ErrNotSupported` — there is no port. |

### The install script contract, minus the port

A tool's script is a normal install script (see
[04 — Install scripts](04-install-scripts.md)) with two differences:

- **There is no `APP_INTERNAL_PORT`.** Nothing is listening, so nothing has to
  bind. The rule about binding `0.0.0.0` does not apply.
- **`healthcheck` is rejected.** It probes a port, and there is none. A tool
  proves it worked by exiting non-zero when it did not — a mount tool waits for
  its mountpoint to appear and fails the install if it never does.

The `service` field is still meaningful and still worth setting: it is what
stop and uninstall act on. A tool that starts something long-running and does
not name its unit cannot be stopped.

### What the Applications tab shows

A tool's installed row has no port row and no credentials panel — showing them
would be showing zeros. It says instead:

> Workspace tool — installed in this project's container. Nothing is exposed.

followed by its non-secret env values, which is where a mount path or a bucket
name shows up. The uninstall confirmation says nothing about releasing a host
port, because none was held.

## `ui`

A UI image installs nothing, anywhere. Its whole payload is the `ui/`
directory; installing it only records that the user switched it on, which is
what makes the SPA load it.

Concretely, installing one:

- creates **no container**, at either scope;
- allocates **no host port** and creates **no proxy device**;
- runs **no install script** — it does not need to have one;
- completes instantly, and works on a host with no container runtime at all.

The instance is stored with `internalPort: 0`, `externalPort: 0`, and an empty
`containerName`, and goes straight to `running`.

### Why the type exists

Before it, adding a button to the Remote interface would have meant declaring a
port you did not want and provisioning a Linux container to serve nothing. The
type is the difference between "provision MySQL" and "add a button".

### What start / stop / uninstall mean

There is no process to signal, so these operate on the record — which is still
meaningful, because the record is what the SPA reads:

| Action | Effect |
|---|---|
| Stop | Status becomes `stopped`; the SPA stops loading the extension, and its buttons disappear. |
| Start | Status becomes `running`; the extension loads again. |
| Uninstall | The record is deleted; the extension's contributions are dropped. |
| Set port | Rejected with `ErrNotSupported` — there is no port. |

Stop is the useful one: it is how a user turns a plugin off without losing it.

### What the Applications tab shows

A UI image's installed row has no port row and no credentials panel — showing
them would be showing zeros. It says instead:

> Interface extension — nothing runs in a container. Its UI is loaded.

and the uninstall confirmation says nothing about host ports, because none is
released.

## `backend`

A backend image installs nothing in a container either. Its payload is the Go
source under `plugin/`, which the server compiles and runs as a child process,
one per installed instance.

Installing one:

- creates **no container**, at either scope;
- allocates **no host port** and creates **no proxy device**;
- runs **no install script**;
- compiles the image's `plugin/` (cached by fingerprint) and starts it;
- works on a host with no container runtime, but needs a **Go toolchain**.

The instance is stored with `internalPort: 0`, `externalPort: 0`, and an empty
`containerName`, exactly as a `ui` image is.

### What start / stop / uninstall mean

Unlike a UI image, there is a real process here, so these move it:

| Action | Effect |
|---|---|
| Stop | The process is killed. Its `DataDir` is kept, and calls report the app is not running. |
| Start | The process is started again, with the same `DataDir`. |
| Uninstall | The process is killed and its `DataDir` is deleted. |
| Set port | Rejected with `ErrNotSupported` — there is no port. |

A server restart needs no sweep: the next call to a plugin starts it.

### What the Applications tab shows

Like a UI image, a backend image's installed row has no port row and no
credentials panel. It says instead:

> Backend extension — a Go plugin runs on the server, not in a container.

See [15 — Backend plugins](15-backend-plugins.md) for the contract, the build,
and the failure modes.

## Choosing a type

```
Does installing it need to run software in a container?
├── yes
│   ├── does anything need to reach it on a port?
│   │   ├── yes → "service"   (declare port.internal and an install script)
│   │   └── no  → "tool"      (install script, project scope, no port)
└── no
    ├── does it need server-side code?  → "backend"   (ship a plugin/ directory)
    └── is it only browser code?        → "ui"        (ship a ui/ directory)
```

Then add `ui/` or `plugin/` to it as needed — neither is restricted to the type
named after it.

## Two axes, not one

`Kind` answers two independent questions, and the two predicates on it are the
ones the rest of the code branches on:

| | `NeedsContainer()` | `NeedsPort()` |
|---|---|---|
| `service` | yes | yes |
| `tool` | yes | no |
| `ui` | no | no |
| `backend` | no | no |

`NeedsContainer` decides whether an install has to reach `lxc` at all.
`NeedsPort` decides whether it allocates a host port and gets a proxy device.
They were the same predicate until `tool` existed; anything that still treats
them as one is a bug waiting for a tool image.
