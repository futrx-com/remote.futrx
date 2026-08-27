# System overview

## What the application is

`remote.futrx` is a self-hosted browser workspace for Claude Code, Codex, Kimi
Code, and Antigravity. Users create project-scoped containers, run interactive
or scheduled agent turns against those projects, and inspect the result through
chat, files, Git, a terminal, an IDE, or a live app preview.

Read [Philosophy](00-philosophy.md) for the design rationale behind project-scoped authority, durable project and provider homes, the host control plane, and the isolation contract.

## Runtime architecture

```mermaid
flowchart LR
    User["Browser user"] -->|"HTTPS"| Caddy["Caddy"]
    Caddy -->|"Main UI, API, WebSockets"| Go["Go backend"]
    Go --> SPA["Embedded Preact SPA"]
    Go --> Stores["JSON and JSONL stores"]
    Go --> Scheduler["Host scheduled-task loop"]
    Go --> Host["Host integrations"]
    Host --> LXD["LXD"]
    Host --> Git["Git CLI"]
    Host --> Tmux["tmux and PTY"]
    Host --> Info["Host resource collector"]

    LXD --> P1["Project container A"]
    LXD --> P2["Project container B"]
    P1 --> Agent["Agent CLI"]
    P1 --> IDE["code-server"]
    P1 --> Apps["Project web apps"]
    P1 --> Chromium["Agent Browser"]

    Caddy -->|"*.code host"| IDE
    Caddy -->|"slug--port.dev host"| Apps
    Caddy -->|"slug--6080.dev host"| Chromium
```

## Application layers

| Layer | Responsibility |
| --- | --- |
| Frontend | Authentication gates, workspace navigation, chat rendering, drawers, settings, and API clients |
| HTTP and WebSocket transport | Routes, JSON responses, upgrades, session checks, and project membership checks |
| Services | Agent module catalog/runtime, auth, capability and execution orchestration, chat, prompt, schedule, project, user, settings, skills, Git, files, browser, and container policy |
| Integrations | LXD, Git, tmux, host filesystem, Google OAuth, host metrics, and container commands |
| Stores | File-backed auth, users, settings, chats, scheduled tasks, projects, access lists, and secrets |
| Infrastructure | Installation, systemd, Caddy, LXD image creation, updates, and recovery timers |

## Main user surfaces

```mermaid
flowchart TD
    Gate["Authentication and onboarding gate"] --> Shell["Workspace shell"]
    Shell --> Sidebar["Project and chat sidebar"]
    Shell --> Chat["Chat view"]
    Shell --> Projects["Project workspace controls"]
    Shell --> Settings["Settings"]

    Chat --> Composer["Provider, model, mode, skills, attachments"]
    Chat --> Messages["Text, reasoning, tools, usage"]
    Chat --> Drawers["History, files, schedules, browser"]
    Chat --> Terminal["Resizable Terminal pane"]

    Projects --> Lifecycle["Start, stop, restart, delete"]
    Projects --> Inspect["Resources, network, agent status"]
    Projects --> Manage["Limits, secrets, sharing"]

    Settings --> Agents["Agent sign-in"]
    Settings --> Appearance["Theme"]
    Settings --> Users["Google OAuth and users"]
    Settings --> Server["Host resource information"]
```

## End-to-end work flow

```mermaid
sequenceDiagram
    actor User
    participant UI as Browser UI
    participant API as Go backend
    participant Prompt as Prompt service
    participant Runtime as module.Runtime
    participant Provider as Provider adapter
    participant Prep as ProjectPreparer
    participant Project as Project service
    participant LXD as LXD container
    participant Agent as Selected agent CLI

    User->>UI: Create a project
    UI->>API: POST /api/projects
    API->>Project: Create metadata and membership
    Project->>LXD: Launch from the base image
    LXD-->>Project: Running workspace
    Project-->>UI: Project upsert over workspace WebSocket

    User->>UI: Create chat and send prompt
    UI->>API: Open /ws/chat/{id}
    UI->>API: prompt message
    API->>Prompt: Start provider-neutral run
    Prompt->>Runtime: Validate scope and lookup provider
    Prompt->>Provider: Run
    Provider->>Prep: Prepare project and selected features
    Prep->>Project: Resolve and start/reconcile
    Project->>LXD: Enforce project lifecycle
    Prep-->>Provider: Prepared target and secrets
    Provider->>Agent: Run or resume native CLI in /workspace
    Agent-->>Provider: Provider-native stream/protocol
    Provider-->>API: Normalized text, reasoning, tool, session, usage events
    API-->>UI: Persisted event stream
    UI-->>User: Render live progress and result

    opt scheduled task
        API->>API: Claim due occurrence
        API->>Project: Re-authorize owner and project
        API->>Prompt: Submit stored prompt through the same run service
        Prompt->>Runtime: Validate scope and lookup provider
        Prompt->>Provider: Run
        Provider-->>API: Normalized run events
        API-->>UI: Same persisted chat transcript when connected
    end
```

## Workspace navigation

The sidebar groups chats under projects and keeps loose chats in a separate section. It supports project/chat search, project reordering, unread and running indicators, new project or chat creation, chat fork/delete, and read/unread toggling. Project start and stop controls live under the project's Settings tab. Selecting a chat closes the mobile sidebar and opens its active `ChatContainer`.

The main shell switches between three views without browser routing:

| View | Main features |
| --- | --- |
| Chat | Streaming thread, composer, terminal, files, Git history, schedules, and browsers |
| Project workspaces | Lifecycle, diagnostics, limits, secrets, and sharing |
| Settings | Provider sign-in, system/dark/light theme, Google users, and server metrics |

## Important boundaries

- The host owns authentication, metadata, HTTPS, access decisions, and container orchestration.
- Each concrete agent adapter owns one factory declaration that binds its
  runtime, authentication, feature policy, provisioning profile, and
  preparation policy. The generic module factory constructs shared project
  preparation and narrows dependencies. Config owns reviewed registration
  order and application-wide policy; the catalog builds one runtime containing
  matching provider and auth registries.
- Each project owns its `/workspace` files and processes.
- Each project also has durable Codex, Claude, Kimi, and Antigravity homes
  mounted at their provider-native paths. Antigravity mounts only
  `/root/.gemini/antigravity-cli`.
- Agent-provider credentials are host-managed and synchronized into project credential locations, primarily those homes; Claude also uses `/root/.claude.json` outside its mounted home.
- Remote's supported Antigravity flow authenticates inside each project and
  stores its current state in the durable
  `/root/.gemini/antigravity-cli` provider mount.
- Scheduled-task definitions and claims live in the host control plane. A due
  task enters the same project chat and one-run-per-chat path as an interactive
  prompt.
- The workspace WebSocket carries project/chat list updates; each chat has its own event stream.
- Caddy authenticates IDE and preview requests before proxying them into containers.

## Code map

- Frontend entry: [`frontend/src/app/App.tsx`](../../frontend/src/app/App.tsx)
- Backend composition root: [`backend/cmd/remote/main.go`](../../backend/cmd/remote/main.go)
- Service composition: [`backend/internal/service/services.go`](../../backend/internal/service/services.go)
- Container composition: [`backend/internal/config/containers.go`](../../backend/internal/config/containers.go)
- Reverse proxy template: [`infra/templates/Caddyfile.tmpl`](../../infra/templates/Caddyfile.tmpl)
