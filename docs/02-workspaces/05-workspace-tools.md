# Workspace tools

The chat header opens Terminal, Files, Git History, Schedules, Browser, and the
project IDE. Most are views over the same project workspace; Schedules is a
host-control-plane view whose runs return to the same chat.

## Tool map

```mermaid
flowchart TD
    Chat["Project chat"] --> Terminal["Resizable Terminal pane"]
    Chat --> Files["File manager"]
    Chat --> History["Git history"]
    Chat --> Browser["Browser drawer"]
    Chat --> IDE["Browser IDE links"]
    Chat --> Schedules["Scheduled tasks drawer"]
    Chat --> Upload["Attachments"]

    Terminal --> Workspace["/workspace"]
    Files --> Workspace
    History --> Repos["Git repositories under workspace"]
    IDE --> CodeServer["Project code-server"]
    Upload --> UploadDir["/workspace/.uploads"]
    Schedules --> ScheduleStore["Host scheduled-task store"]
```

## Attachments

Files can be selected, dragged, or pasted into the composer. Uploads use the resumable tus protocol.

```mermaid
sequenceDiagram
    actor User
    participant UI as Composer
    participant Tus as Upload API
    participant Temp as Disk-backed chunk store
    participant Chat as Chat service
    participant WS as Project workspace

    User->>UI: Add one or more files
    UI->>Tus: POST metadata with chat ID and filename
    Tus->>Tus: Check session and project access
    Tus-->>UI: Random upload URL
    loop 5 MiB chunks
        UI->>Tus: PATCH chunk and offset
        Tus->>Temp: Append chunk
    end
    Tus->>Chat: Resolve stable project upload directory
    Tus->>WS: Move file to .uploads without overwrite
    UI->>UI: Add saved paths to prompt text
    UI->>WS: Send prompt referencing paths
```

Important behavior:

- Default maximum upload size is 10 GiB and can be changed with `UPLOAD_MAX_BYTES`.
- Chunks live under the application data directory, not RAM-backed `/tmp`.
- Final files are mode `0644` and owned by the container-mapped root user.
- `.uploads/.gitignore` ignores every attachment.
- Existing filenames are not overwritten.

## File manager

```mermaid
flowchart LR
    Open["Open Files drawer"] --> Root["List workspace root"]
    Root --> Expand["Lazy-load directories"]
    Root --> Search["Recursive filename search"]
    Expand --> Download["Download file"]
    Expand --> Zip["Download folder as ZIP"]
    Expand --> Media["Open supported media inline"]
    Expand --> IDE["Open path in IDE"]
```

The backend resolves all paths relative to the chat working directory and
rejects traversal. Listings and search results can report truncation rather
than returning unbounded data.

Selecting a file routes by type:

- supported image/audio/video/PDF opens in the full-screen media overlay;
- code, data, text, logs, and unknown non-media files redirect to code-server;
  and
- archives and unsupported media download.

The same routing applies to validated absolute workspace links in chat. IDE
targets preserve optional `:line[:column]` suffixes. Inline media receives a
restrictive content security policy.

## Terminal

```mermaid
sequenceDiagram
    actor User
    participant UI as xterm.js workspace pane
    participant WS as /ws/terminal
    participant Project as Project service
    participant LXD
    participant Bash as Login shell

    User->>UI: Open terminal
    UI->>WS: Connect with chat ID
    WS->>WS: Check project membership
    WS->>Project: Start project if needed
    Project->>LXD: Ensure container is running
    WS->>LXD: lxc exec with PTY
    LXD->>Bash: cd to mapped chat cwd or /workspace
    UI->>Bash: Input and resize messages
    Bash-->>UI: Binary PTY output
```

The terminal exists only for chats attached to a project. Opening it starts one
interactive `bash -l` process for that loaded chat. On desktop it is a
resizable workspace pane beside the chat; its width is retained in browser
`localStorage`. Terminal, Files, History, Schedules, and Browser panes are
mutually exclusive. Hiding and reopening Terminal in the same loaded chat
keeps the socket, shell, and unsubmitted input alive. Switching chats,
reloading or closing the page, or losing the socket tears the PTY down; it is
not a persistent tmux session.

The backend also retains lower-level tmux session APIs and a `/ws` tmux PTY bridge. These are not used by the current main workspace UI, but chats can still carry a `tmuxSession` and resolve their working directory from it.

## Git history

The History drawer discovers Git repositories at the workspace root and up to a bounded depth, excluding heavy or generated directories.

```mermaid
flowchart TD
    Open["Open History drawer"] --> Discover["Discover repositories"]
    Discover --> Select["Select repository"]
    Select --> Log["Load commits from all refs"]
    Log --> Diff["Inspect commit patch"]
    Diff --> Restore["Restore selected commit"]
    Restore --> Dirty{"Working tree dirty?"}
    Dirty -->|"No"| Checkout["Detached checkout"]
    Dirty -->|"Yes, no checkpoint requested"| Stop["Return conflict and dirty files"]
    Dirty -->|"Yes, checkpoint requested"| Commit["Stage all and create checkpoint commit"]
    Commit --> Checkout
```

Restore resolves the commit, optionally creates a safety checkpoint using the `remote.futrx` identity, and checks out the target in detached HEAD state. Git commands use an explicit safe directory and bounded timeouts.

The frontend checks for repositories when a ready chat opens and after each
run; **History** stays hidden until at least one exists. Commit patches render
as collapsible per-file cards with line numbers, hunks, change counts, and file
status badges, with raw-text fallback.

The checkout API supports the checkpoint message, but the current History drawer does not render the dirty-tree checkpoint form even though its component state is present. In the visible UI, commit or stash dirty work through Terminal or IDE, refresh History, and then switch. Clean-tree switching works directly.

## Scheduled tasks

The Schedules drawer is project-chat-only. It lists host-owned tasks visible to
the caller, then updates them through the schedule API. The drawer can arm,
pause/resume, run-now, edit, delete, and refresh tasks. Initial creation in the
current UI is agent-driven through the **Scheduled Tasks** skill.

Schedule timers do not run inside the container. The backend persists claims,
starts the project if needed, and injects the stored prompt through the normal
chat run path. See [Scheduled tasks](06-scheduled-tasks.md).

## Browser IDE

Each project has an on-demand code-server instance on container port `8842`.

```mermaid
flowchart LR
    Link["Open IDE or file link"] --> Auth["Caddy forward-auth"]
    Auth --> Host["code.<host>/<slug>/ or <slug>.code.<host>"]
    Host --> Socket["In-container socket activation"]
    Socket --> Code["code-server"]
    Code --> Workspace["/workspace"]
```

Caddy disables upstream keep-alive so code-server can stop after its idle window. Platform session cookies are removed before requests reach the container.

## IDE and media links in chat

Markdown links are inspected by the frontend. Workspace paths can be converted
into backend `ide-open` or `media-open` routes. The backend validates the path,
then either redirects to code-server—using the workbench payload for an exact
file and optional line/column—or serves supported image/audio/video/PDF media
in the in-app viewer.

## Code map

- Upload handler: [`backend/internal/transport/http/upload_tus.go`](../../backend/internal/transport/http/upload_tus.go)
- Workspace files: [`backend/internal/service/workspacefiles/service.go`](../../backend/internal/service/workspacefiles/service.go)
- Terminal socket: [`backend/internal/transport/ws/container_terminal_socket.go`](../../backend/internal/transport/ws/container_terminal_socket.go)
- Git history: [`backend/internal/service/githistory/service.go`](../../backend/internal/service/githistory/service.go)
- IDE service: [`backend/internal/service/workspaceide/service.go`](../../backend/internal/service/workspaceide/service.go)
- Schedules UI: [`frontend/src/ui/chat/schedules/ScheduleDrawer.tsx`](../../frontend/src/ui/chat/schedules/ScheduleDrawer.tsx)
