# Deployment and operations

The supported deployment is a root-managed Ubuntu or Debian server with DNS pointing to the host, ports 80 and 443 open, and working SSH key access.

## Installation flow

```mermaid
flowchart TD
    Start["Run infra/install.sh with hostname"] --> Root["Validate root and options"]
    Root --> Checkout["Select target checkout and re-execute its installer"]
    Checkout --> Validate["Validate distro and DNS"]
    Validate --> Deps["Install pinned host dependencies"]
    Deps --> Agents["Converge catalog-declared host agent CLIs"]
    Agents --> Build["Build frontend and Go backend"]
    Build --> Proxy["Render, validate, and reload Caddy"]
    Proxy --> Service["Install and start systemd backend service"]
    Service --> Health["Poll backend health for up to 30 seconds"]
    Health --> Image["Build reusable LXD workspace image"]
    Image --> SSH["Disable SSH password authentication"]
    SSH --> Heal["Enable minutely LXD IPv4 repair timer"]
    Heal --> Ready["Open app and claim administrator"]
```

The curl bootstrap installs Git if needed, clones into `/opt/remote.futrx`, and
re-executes the checked-out installer. Direct repair runs follow the same
select-and-re-execute rule before reading version pins or the agent catalog, so
one convergence cannot mix policy from two commits.

## Installed components

| Component | Purpose |
| --- | --- |
| `/opt/remote.futrx` | Application checkout, built binary, frontend assets, infrastructure scripts, and data |
| `remote.futrx.service` | Go backend on loopback port `7682` by default |
| Caddy | Public HTTPS, compression, authentication, and proxy routing |
| LXD | Project-container runtime and base-image store |
| Catalog-declared host agent CLIs | Local binaries for host-scoped execution and managed authentication |
| `futrx-remote-dev-base` | Reusable Ubuntu workspace image |
| `.lxd` DNS integration | Resolves container names through the LXD bridge |
| `lxc-ipv4-heal.timer` | Repairs running containers that lose IPv4 |
| Main application PWA | Installable chat/control surface, Web Push, and a network-failure offline page |
| code-server launcher PWA | One installable entry point for project IDEs |

## Build flow

```mermaid
flowchart LR
    Frontend["Preact and TypeScript source"] --> Vite["tsc and Vite build"]
    Vite --> Public["backend/public embedded assets"]
    GoSource["Go source"] --> GoBuild["go build"]
    Public --> Binary["Single remote binary"]
    GoBuild --> Binary
    Binary --> Systemd["remote.futrx.service"]
```

The backend embeds the compiled frontend, so Caddy only needs to proxy the main origin to the Go process.

## Host agent CLI convergence

After the host toolchain is available and the target checkout is selected, the
application step builds the explicit compiled-in agent module catalog and runs
`backend/cmd/install-host-agents`, which selects profiles for modules declaring
the `host` execution scope. A host-scoped local CLI module supplies a profile;
a host-only remote integration may omit one and requires no local install.
Each profile originates from the provider's local `NewFactory()`/`Profile()`;
the installer applies the catalog without provider-specific branches.

For each selected profile, the installer runs the provider-declared version
arguments against its application-managed executable under
`/opt/remote.futrx/data/host-clis/bin` and compares the detected semver with the
exact pin (or checks binary existence when version checks are disabled). npm
and standalone-script installers target the same managed prefix. The installer
and backend service put that directory first on `PATH`, and convergence rejects
any state where ordinary command resolution selects a different executable.
The global
`config.AgentOptions.HostCLIVersionTimeout` currently caps each host version
command at 15 seconds. The installer
installs stale/missing npm CLIs at the exact package pin or runs the profile's
pinned install script, then applies the profile's post-install verification
policy with a fresh version-probe budget from that same global setting. The
provider-declared profile timeout bounds only the mutating install command.
Providers are converged sequentially because they share the managed prefix.
Each install runs in an
isolated process group; cancellation terminates its descendants before the
updater returns. Any provider-scoped install or verification error aborts the
infrastructure step. Preview the derived plan without changing the host:

```bash
cd /opt/remote.futrx/backend
go run ./cmd/install-host-agents --plan
```

The same profile CLI policy is consumed by the project base-image and runtime
repair paths, preventing a second provider list from drifting away from host
execution. A CLI pin or module-profile change therefore requires the full
infrastructure update; the application-only deployer does not rerun host
convergence or rebuild project images.

## Public routing

```mermaid
flowchart TD
    Internet["Internet"] --> Caddy
    Caddy --> Main["host → Go backend"]
    Caddy --> Launcher["code.host → IDE launcher or slug path"]
    Caddy --> ProjectIDE["slug.code.host → slug.lxd:8842"]
    Caddy --> Preview["slug--port.dev.host → slug.lxd:port"]
    Caddy --> Inspector["preview /__remote_inspector → Go backend"]

    ProjectIDE --> Auth["forward_auth /auth/verify"]
    Preview --> Auth
    Launcher --> Auth
    ProjectIDE --> TLS["on-demand TLS checked by /internal/tls-ask"]
    Preview --> TLS
```

Caddy validates its rendered configuration before replacing the live file. On-demand certificate requests are accepted only for existing project slugs and permitted hostname formats.

## Base-image build

```mermaid
sequenceDiagram
    participant Builder as build-base-image
    participant LXD
    participant Ubuntu as Ubuntu 24.04 builder
    participant Alias as futrx-remote-dev-base

    Builder->>LXD: Delete leftover builder if present
    Builder->>Ubuntu: Launch temporary container
    Builder->>Ubuntu: Install system tools, Node, GitHub CLI, catalog-declared project CLIs
    Builder->>Ubuntu: Install Chromium and Agent Browser
    Builder->>Ubuntu: Install code-server
    Builder->>Ubuntu: Stop container
    Builder->>Alias: Publish reusable image
    Builder->>LXD: Remove temporary builder
```

The recipe is generated from the project-scoped module profiles used by
runtime CLI repair. Host convergence uses the host-scoped subset of the same
catalog, keeping provider packages and pins consistent across execution
environments.

## Update flow

```mermaid
flowchart TD
    Check["In-app updater finds newest release tag"] --> Line{"Installed and target major/minor match?"}
    Line -->|"Yes: patch release"| App["Run infra/deploy-app.sh"]
    App --> AppBuild["Build frontend/backend into a staged binary"]
    AppBuild --> AppRestart["Replace binary, restart, and health-check"]
    AppRestart --> AppDone["Keep host, base image, and containers unchanged"]
    Line -->|"No: major/minor release"| Update["Run infra/update.sh"]
    Update --> Pull["Fetch and reset installed checkout to release tag"]
    Pull --> Reexec["Re-execute the new updater"]
    Reexec --> Install["Converge dependencies, rebuild, restart, and health-check"]
    Install --> Rebuild["Rebuild base image"]
    Rebuild --> Scan["Read project container names from metadata"]
    Scan --> Busy{"Active lxc exec agent process?"}
    Busy -->|"Yes, default"| Skip["Skip busy container"]
    Busy -->|"No"| Delete["Delete replaceable container"]
    Delete --> Relaunch["Next start or prompt relaunches from new image"]
    Relaunch --> Mount["Reattach persistent workspace and provider homes"]
    Mount --> Provision["Reprovision tools and compatibility links"]
```

Release tags use `MAJOR.MINOR.PATCH`. Crossing a major or minor boundary runs
the full infrastructure updater; movement within one major/minor line runs the
application-only deployer. The decision is relative to the installed version:

| Upgrade | Deployment path |
| --- | --- |
| `0.3.1` → `0.3.2` | Application only |
| `0.3.1` → `0.4.0` | Infrastructure |
| `0.3.1` → `0.4.2` | Infrastructure, because the host missed the `0.4` baseline |
| `0.4.0` → `0.4.2` | Application only |

The application deployer stages the new binary, restores the previous checkout
and binary when restart or health verification fails, and refuses to cross a
major/minor boundary. Unknown and legacy two-component installed versions take
the conservative infrastructure path.

`--include-busy` forces busy workspace recycling. `--skip-workspaces` updates only the host and application. `upgrade-workspaces.sh --dry-run` shows the workspace plan without changing it.

The intended default is to skip active agent containers. The current busy-process matcher expects a different `lxc exec` argument order than the provider commands use, so it may classify an active run as idle. Until that detector is corrected, treat workspace recycling as disruptive: coordinate a maintenance window or use `--skip-workspaces` while runs are active.

The updater intentionally resets the installed application checkout to the
requested tag/ref, defaulting to `origin/main` when none is supplied. The
in-app updater supplies its selected release tag. Persistent application data
and project workspaces live outside the tracked source tree.

## Startup reconciliation

When the backend starts, it:

1. loads file stores and in-memory indexes;
2. builds the agent and container service graph;
3. compares project metadata with LXD state;
4. updates stored project status;
5. reapplies the fleet resource profile and project overrides;
6. starts the Agent Browser idle reaper;
7. starts the scheduled-task loop and restores persisted deadlines/claims;
8. begins serving the embedded SPA, API, and WebSockets.

## Agent capability discovery timeout

Capability discovery probes all registered agents compatible with the selected
host/project execution scope concurrently. One global
deadline applies to each provider's complete probe, including any primary and
fallback commands it runs:

| Environment variable | Default | Meaning |
| --- | ---: | --- |
| `AGENT_CAPABILITY_TIMEOUT` | `30s` | Maximum duration of one provider capability probe; Go duration syntax; `0` disables the deadline |

Set it with a systemd override when slower provider CLIs need more time:

```ini
[Service]
Environment=AGENT_CAPABILITY_TIMEOUT=45s
```

Restarting the service applies the value and also clears the process-local
capability cache. Invalid or negative values fall back to 30 seconds.

## Scheduled-task guardrails

Scheduled tasks are host-owned unattended runs, so the backend applies three
independent limits:

| Environment variable | Default | Meaning |
| --- | ---: | --- |
| `SCHEDULE_MIN_INTERVAL` | `5m` | Minimum time between starts of one recurring task; Go duration syntax |
| `SCHEDULE_MAX_CONCURRENT` | `2` | Simultaneous scheduled runs across all chats |
| `SCHEDULE_MAX_TASKS_PER_PROJECT` | `20` | Non-terminal standing tasks in one project |

An explicit `0` disables a limit. **Run now** bypasses the interval and
concurrency admission limits, but the forced run still counts while active.
Terminal completed/exhausted/error definitions do not consume the
per-project task quota.

Create a systemd override rather than editing the installed unit template:

```bash
sudo systemctl edit remote.futrx
```

```ini
[Service]
Environment=SCHEDULE_MIN_INTERVAL=10m
Environment=SCHEDULE_MAX_CONCURRENT=1
Environment=SCHEDULE_MAX_TASKS_PER_PROJECT=10
```

Then apply it:

```bash
sudo systemctl daemon-reload
sudo systemctl restart remote.futrx
```

Restarting the backend interrupts control of interactive and scheduled runs.
Use a maintenance window. Before raising the limits, account for the fact that
each scheduled occurrence can start a project container and consume provider
quota, CPU, memory, network, and disk without an open browser.

## Health and recovery

```mermaid
flowchart LR
    Backend["Backend restart"] --> Reconcile["Container status and limits reconcile"]
    Timer["Every minute"] --> MissingIP{"Running container has no IPv4 after boot grace?"}
    MissingIP -->|"Yes"| Reconfigure["networkctl reconfigure eth0"]
    UI["Manual Repair network"] --> Reconfigure
    Reconfigure --> Inspect["Reinspect for IPv4 up to five times"]
```

The server-info settings page reports host, CPU, memory, storage, network, and Go-process metrics. The project page reports the corresponding per-container diagnostics.

## Security controls

- The backend listens on loopback by default; Caddy is the public entry point.
- Platform sessions use secure, HTTP-only cookies.
- Preview and IDE requests use forward authentication; preview authorization is project-aware, while IDE authorization currently accepts any registered user.
- Platform cookies are removed before container proxying.
- Internal Caddy helper routes are denied externally.
- Secret, auth, access, and user files use restrictive permissions.
- SSH password and keyboard-interactive authentication are disabled after install.
- On-demand TLS issuance is restricted to valid, existing project hosts.
- Project containers are unprivileged and receive host workspaces through mapped ownership.
- Project containers currently share the LXD bridge without lateral ACLs; code-server and noVNC rely on Caddy for public authentication and do not independently authenticate direct bridge traffic.

## Operational commands

```bash
systemctl status remote.futrx
systemctl status caddy
journalctl -u remote.futrx -f
sudo bash /opt/remote.futrx/infra/update.sh
sudo bash /opt/remote.futrx/infra/deploy-app.sh --ref=0.4.2
sudo bash /opt/remote.futrx/infra/upgrade-workspaces.sh --dry-run
```

## Code map

- Installer: [`infra/install.sh`](../../infra/install.sh)
- Application deployer: [`infra/deploy-app.sh`](../../infra/deploy-app.sh)
- Updater: [`infra/update.sh`](../../infra/update.sh)
- Workspace upgrade: [`infra/upgrade-workspaces.sh`](../../infra/upgrade-workspaces.sh)
- Systemd template: [`infra/templates/remote.futrx.service.tmpl`](../../infra/templates/remote.futrx.service.tmpl)
- Base-image builder: [`backend/internal/service/container/image/builder.go`](../../backend/internal/service/container/image/builder.go)
