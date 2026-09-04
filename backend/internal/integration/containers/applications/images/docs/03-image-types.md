# 03 — Image types

`type` in `image.json` decides what installing an image actually *does*, and in
particular whether it touches a container at all.

| type | Installing it | Requires |
|---|---|---|
| `service` (default) | Runs software on a port under systemd | `port.internal`, an install script |
| `ui` | Nothing in any container — turns on the image's browser extension | a `ui/` directory |

The Go type is `Kind` in
[`service/applications/model.go`](../../../../../service/applications/model.go).
Omitting `type` means `service`, so every image written before this field
existed keeps working unchanged.

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
| Start | same as install (the script is idempotent) | same as install |
| Stop | remove the proxy, `lxc stop --force` the container | remove the proxy, `systemctl stop <service>` only |
| Uninstall | `lxc delete --force` the container | remove the proxy, `systemctl disable --now <service>`; packages and data stay |

Stopping a project app must never stop the container someone is working in, and
uninstalling one must never delete their project. Both are pinned by tests.

### Reaching a service

Every installed service is reachable on the host through an LXD **proxy
device**:

```
listen  = <protocol>:<bindAddress>:<externalPort>   on the host
connect = <protocol>:127.0.0.1:<internalPort>       inside the container
```

A project app is additionally reachable from inside the project's own container
on the LXD bridge at `<slug>.lxd:<internalPort>`.

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

## Choosing a type

```
Does installing it need to run software in a container?
├── yes → "service"        (declare port.internal and an install script)
└── no  → "ui"             (ship a ui/ directory; declare no port)
```

## The gap: install script, no service

There is currently no type for "run an install script in the project's
container, but expose no port and no systemd unit" — installing a CLI tool into
a workspace, say. Modelling that as `service` forces a port you do not want.

If you need it, the shape would be a third kind (`tool`: install script,
project scope only, no port or unit) and it is a small addition to
`Kind`, `validate`, and `Service.Install`. It has not been added because
nothing in the catalog needs it yet.
