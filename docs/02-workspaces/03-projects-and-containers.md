# Projects and containers

A project is a durable workspace, the persistent provider state declared by
registered project-capable agent modules, and an LXD container that supplies
their processes and tools. The durable directories survive container rebuilds;
the container can be replaced.

## Project creation

```mermaid
sequenceDiagram
    actor User
    participant API as Project API
    participant Store as Project store
    participant Access as Access store
    participant Files as Host filesystem
    participant LXD
    participant Provision as Launch provisioners

    User->>API: Create project with a name
    API->>Store: Create ID, unique slug, metadata
    API->>Access: Add creator as a member
    API->>Files: Prepare durable workspace and agent-home directories
    API->>LXD: Launch from futrx-remote-dev-base
    LXD->>LXD: Attach workspace and provider homes
    API->>Provision: Credentials, skills, browser assets, code-server
    API->>Store: Mark running or error
    Store-->>User: Workspace WebSocket project update
```

The slug becomes the container name and is used in IDE and preview hostnames.
Display names are unique case-insensitively after trimming whitespace; create
and rename reject a duplicate with `409 Conflict`. Distinct display names that
normalize to the same slug receive `-2`, `-3`, and later suffixes.

## Durable and replaceable parts

```mermaid
flowchart LR
    Project["Project"] --> Meta["Project metadata"]
    Project --> Access["Membership list"]
    Project --> Secrets["Secret store"]
    Project --> Workspace["Host workspace directory"]
    Project --> Homes["Host provider agent homes"]
    Project --> Container["LXD container"]

    Workspace --> Mount["Bind-mounted at /workspace"]
    Homes --> Codex["Mounted at /root/.codex"]
    Homes --> MiniMax["Mounted at /root/.minimax"]
    Homes --> Claude["Mounted at /root/.claude"]
    Homes --> Kimi["Mounted at /root/.kimi-code"]
    Homes --> Antigravity["Mounted at /root/.gemini/antigravity-cli"]
    Container --> RootFS["Replaceable root filesystem"]
    Container --> Tools["Agent CLIs, code-server, Chromium"]
    Container --> Processes["Agent, terminal, and app processes"]
```

Files in `/workspace` and every registered provider-state mount survive stop,
restart, container deletion during upgrades, and image replacement. Provider
homes preserve most provider-owned configuration, authentication, and session
state.
Antigravity persists only `/root/.gemini/antigravity-cli`, not the rest of
`/root/.gemini`. Claude also uses `/root/.claude.json` outside its mounted home
and relies on host credential synchronization to restore it. Ad-hoc packages or
files elsewhere in the container root filesystem do not survive container
replacement.

## Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Provisioning: create
    Provisioning --> Running: launch succeeds
    Provisioning --> Error: launch fails
    Running --> Stopped: stop
    Stopped --> Running: start
    Running --> Running: restart
    Stopped --> Running: restart
    Missing --> Running: start or restart relaunches
    Error --> Running: successful start or restart
    Running --> Missing: container removed outside metadata
    Running --> [*]: admin deletes project
    Stopped --> [*]: admin deletes project
```

At backend startup, reconciliation compares stored status with actual LXD state and reapplies the default and per-project resource envelope.

## Container launch contents

Project container policy is module-driven. Only modules declaring the
`project` execution scope contribute a provisioning profile. Each provider's
local `NewFactory()` passes its `Profile()` separately from the public module
descriptor; the shared catalog and provisioning services never reconstruct
provider policy. The factory retains a defensive profile snapshot and passes
independent exact validated clones to its shared project preparer and provider
build callback. That profile owns the CLI install/repair spec, credential
synchronization, persistent-state mounts, shared instructions, workspace-skill
compatibility, non-secret runtime assets, and optional browser MCP templates. A project-capable module
without a complete matching profile is rejected when the module catalog is
built. Shared workspace provisioners converge all catalog-configured
instruction and skill targets rather than only the currently selected
provider's target.

Profiles may also publish non-secret runtime assets for the selected agent.
MiniMax uses this path for its Codex model catalog; its Token Plan subscription key remains a
project secret and is never written into that template.

The reusable Ubuntu 24.04 base image contains:

- Node.js 22, Git, SSH client, `jq`, build tools, Python, and GitHub CLI.
- Claude Code, Codex, Kimi Code, and Antigravity at pinned versions. MiniMax
  reuses the pinned Codex CLI with an isolated provider configuration.
- The Agent Browser stack and Chromium.
- `code-server` with on-demand startup.

Launch-time provisioning then:

1. Seeds or synchronizes registered agent credentials into project credential locations, primarily the durable provider homes.
2. Links agent skill directories into the workspace.
3. Publishes current browser scripts and browser skill.
4. Applies browser process limits.
5. Configures the project IDE.

When a prompt selects **Scheduled Tasks**, Remote also publishes the
provider-neutral `remote-schedule` CLI and skill under `/workspace` before
starting the provider.

These launch steps are best-effort so one optional capability does not prevent the container from starting.

Before each selected-provider run, shared preparation verifies that provider's
CLI, converges instructions and its runtime assets, refreshes skill links,
and prepares any requested Browser or Scheduled Tasks capability. Required
steps fail the run with a specific preparation error.

## Start and restart behavior

```mermaid
flowchart TD
    Start["Start request or prompt needs project"] --> Ensure["Run lifecycle convergence"]
    Ensure --> State{"Container exists?"}
    State -->|"No"| Init["Initialize stopped container from base image"]
    State -->|"Yes"| Inspect["Inspect state and required mounts"]
    Init --> Attach["Attach workspace and registered provider homes"]
    Inspect --> Missing{"Any provider-home mount missing?"}
    Missing -->|"Yes"| Migrate["Recover legacy provider state, stopping the container when needed"]
    Migrate --> Attach
    Missing -->|"No"| Limits["Apply resource profile and project overrides"]
    Attach --> Limits
    Limits --> Running{"Container running?"}
    Running -->|"No"| Resume["Start or restart container"]
    Running -->|"Yes"| Verify["Verify all required mounts are active"]
    Resume --> Verify
    Verify --> Changed{"New container or mount topology changed?"}
    Changed -->|"Yes"| Provision["Run best-effort launch provisioners"]
    Changed -->|"No"| Ready["Ready for work"]
    Provision --> Recreated{"Newly recreated container?"}
    Recreated -->|"Yes"| Env["Best-effort restore project secrets"]
    Recreated -->|"No"| Ready
    Env --> Ready
```

Restart is host-driven and can recover a container whose internal processes are wedged. If the container is missing, restart delegates to the full launch path. A legacy container missing provider-home mounts may be started briefly to copy old provider state out, then stopped and reattached to the converged layout.

## Secrets

Project secrets are validated environment keys with an authoritative host-side store. They are also mirrored into the managed workspace `.env` file for project tools.

```mermaid
flowchart TD
    UI["Secrets UI"] --> Store["Encrypted-at-rest is not provided; file is mode 0600"]
    Store --> DotEnv["All values in generated /workspace/.env"]
    Store --> AgentExec["All values passed explicitly to agent executions"]
    Store --> LXDEnv["Single-line values in LXD environment.KEY"]
    LXDEnv --> NewExec["New terminal and other inherited processes"]
    DotEnv --> App["Project applications using dotenv"]
```

Adding or updating a secret commits the authoritative store, then attempts to write the managed `.env` file and update the container configuration. Multiline values are deliberately omitted from LXD environment configuration but are still passed to agent executions and written to `.env`. Deleting a secret removes the authoritative entry and attempts to remove each managed copy. Those propagation steps are best-effort and log rather than roll back on failure, so a stale copy can remain. Already-running processes retain their old environment until restarted.

## Sharing

- Project creators are added to the membership list automatically.
- Any current member can list members and add a registered user.
- Any current member can remove a user, but a non-admin cannot remove the final member.
- Administrators bypass membership checks and can always recover access.

## Resources and inspection

The project workspace page provides:

| Control or data | Details |
| --- | --- |
| Lifecycle | Start, stop, restart, and admin-only delete |
| Resource limits | CPU, memory, and root-filesystem disk overrides; admin-only changes |
| Runtime resources | Processes, CPU time, current/peak memory, swap, and disk usage |
| Container identity | Image, type, architecture, PID, creation, and last use |
| Network | Interfaces, addresses, traffic, MAC, and MTU |
| Operating system | Distribution, kernel, uptime, CPU count, and hostname |
| Agent state | Installed versions, instructions, and credential-bundle freshness |
| Recovery | Manual network repair and automatic IPv4 repair timer |

Fleet CPU, memory, and process defaults are reapplied best-effort: an application failure is currently ignored. Explicit project overrides fail the launch when they cannot be applied. The optional disk override limits the container root disk only; the host bind mounts for `/workspace` and provider homes have no project quota.

## Port discovery

The backend runs `ss` inside a running container and returns non-loopback TCP listeners on allowed ports from 1024 through 65535. The browser drawer turns a selected listener into a `slug--port.dev` URL.

## Deletion

The current backend deletion path removes the LXD container, project metadata, the host project root containing both workspace and agent homes, project secrets, and project access list. Chat records are stored separately; the project service does not currently cascade-delete chats that reference the deleted project.

## Code map

- Project policy: [`backend/internal/service/project/service.go`](../../backend/internal/service/project/service.go)
- Container lifecycle: [`backend/internal/service/container/lifecycle/service.go`](../../backend/internal/service/container/lifecycle/service.go)
- Project handler: [`backend/internal/transport/http/handlers/project_handler.go`](../../backend/internal/transport/http/handlers/project_handler.go)
- Project UI: [`frontend/src/ui/projects/ProjectContainersPage.tsx`](../../frontend/src/ui/projects/ProjectContainersPage.tsx)
