# 03 — Application capabilities

Every package is an application. There is no `type` field and no distinction
between service, tool, UI, or application backends. Remote discovers what an
application does from its files and manifest fields.

| Capability | How it is detected | Effect when installed |
|---|---|---|
| Infrastructure | `infra/install.sh` exists, `install` names another script inside `infra/`, `backend/container/` exists, or `service` is declared | Provisions the target container |
| Network port | Infrastructure exists and `port.internal` is greater than zero | Allocates a host port and creates an LXD proxy device |
| Backend | `backend/main.go` exists (`backend/api/` as an executable is accepted for compatibility) | Generates one host module from the root and child host packages, excludes `backend/container/`, and runs the backend executable |
| Background backend | `backend.background: true` | Recovers running installations after host restarts and child crashes |
| Agent execution | `backend.agentTurns: true` | Supplies installation-scoped start/read/forget for normal agent turns |
| Agent tools | `backend.agentTools: true` | Supplies scoped access to application-defined backend commands during agent turns |
| UI | `ui/` exists | Loads the browser extension |
| Skills | `skills/*/SKILL.md` exists | Publishes the skills into the target project |

Capabilities compose freely. A single application may provision software,
expose it on a port, run a backend, extend the UI, and publish skills. Removing
one folder removes the capability discovered from it. Backend runtime controls
are declared separately in the manifest; there is no application type
discriminator to keep in sync with the package layout.

Directories such as `backend/api/` and `backend/lifecycle/` are packages within
the backend capability, not capabilities of their own. `backend/main.go`
imports and composes them, and Remote runs the result as one per-instance
process.

## Agent and background controls

These flags require a host backend, are independent, and default to `false`:

| Manifest flag | Application API |
|---|---|
| `backend.agentTurns` | `Runtime.AgentTurns.Start(request)`, `Read(query)`, and `Forget(requestID)`, bound before `Init` through `rpc.ServeWithRuntime` |
| `backend.agentTools` | Agent calls to application-defined routes through `/agent-api/applications/<application-id>/<path>`, with Remote-stamped `Request.Caller` and `Request.Agent` |
| `backend.background` | Restores running backend processes at server startup and on the 15-second recovery sweep; stopped installations remain stopped |

Turns inherit the existing chat's provider, model, and settings. Applications
keep job definitions, timing, retries, and result acknowledgments in their own
`DataDir`. Remote keeps installation-scoped execution receipts and enforces
owner/project access. See [25 — Application agent runtime](25-application-agent-runtime.md).

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
  optional manifest-owned systemd service;
- the backend process starts and stops with the application;
- the UI loads only while the installed instance is running;
- a proxy device exists only when `port.internal` is declared.

For project-scoped infrastructure, uninstall disables the declared service and
removes Remote-owned service files. A declared `uninstall` script can also purge
application packages and data; without it those files remain. The project
container itself is retained. Global uninstall deletes the dedicated container
and skips the cleanup script. See [04 — Install scripts](04-install-scripts.md#uninstall-scripts).

Running applications with container capabilities are reinstalled after project
container replacement; stopped ones remain stopped. A later Start reinstalls
when its declared service-unit check fails. See
[Container recovery](24-application-container-recovery.md).

Background backend and agent capabilities are independent opt-ins. See
[25 — Application agent runtime](25-application-agent-runtime.md) for authority,
receipts, tool permissions, and recovery.
