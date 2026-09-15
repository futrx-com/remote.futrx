# 03 — Application capabilities

Every package is an application. There is no `type` field and no distinction
between service, tool, UI, or backend plugins. Remote discovers what an
application does from its files and manifest fields.

| Capability | How it is detected | Effect when installed |
|---|---|---|
| Infrastructure | `infra/install.sh` exists, or `install` names another script inside `infra/` | Provisions the target container |
| Network port | Infrastructure exists and `port.internal` is greater than zero | Allocates a host port and creates an LXD proxy device |
| Backend | `backend/` exists | Compiles and runs the Go backend on the host |
| UI | `ui/` exists | Loads the browser extension |
| Skills | `skills/*/SKILL.md` exists | Publishes the skills into the target project |

Capabilities compose freely. A single application may provision software,
expose it on a port, run a backend, extend the UI, and publish skills. Removing
one folder removes only that capability; no manifest discriminator needs to be
kept in sync with the package layout.

## Infrastructure and scope

At global scope, infrastructure runs in a dedicated container named
`futrx-app-<instanceID>`. At project scope, it runs in the project's existing
container. Infrastructure without `port.internal` is valid and creates no proxy
device; this is suitable for CLIs, mounts, agents, and background jobs.

Applications without infrastructure create no container. Installing them
records that they are enabled, starts their backend when present, and makes
their UI and skills available.

## Lifecycle

Start, stop, and uninstall operate on every capability an application has:

- infrastructure is started or stopped through its target container and
  optional systemd service;
- the backend process starts and stops with the application;
- the UI loads only while the installed instance is running;
- a proxy device exists only when `port.internal` is declared.

For project-scoped infrastructure, uninstall disables the declared service but
does not delete the project container or files installed into it. For global
infrastructure, uninstall deletes the dedicated application container.
