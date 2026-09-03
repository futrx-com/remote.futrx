# Troubleshooting

Start with the symptom below. Use **Refresh** after a recovery action so that the UI does not show an older snapshot.

## Sign-in and onboarding

### The server asks me to create an admin

No local administrator has claimed the installation yet. Create the first account with the intended owner email and a password of at least 12 characters. The claim endpoint is public only while the server is unclaimed.

### A member sees “waiting for administrator”

The host has not completed local-admin setup or no configured access-gate agent
is ready. A server admin must finish the displayed module access flow. Members
cannot configure host-wide managed provider credentials.

### Google sign-in returns access denied

Check, in order:

1. Google OAuth is enabled under **Settings → Users**.
2. The callback URL exactly matches the configured OAuth client.
3. The person's exact email exists in the Remote user directory.
4. The OAuth client is permitted to serve that account.

Remote has no self-sign-up path.

### I lost the local-admin password

There is no password-reset screen. Host access is required to repair the local-admin record. Follow the operator procedure rather than deleting files from the browser. Rotating the global session key signs every browser out; individual session revocation is not available.

## Projects and containers

### Project creation stays on provisioning

1. Open the project gear and inspect any visible error.
2. Refresh the workspace.
3. Check host capacity under **Settings → Info**.
4. Confirm that the base image exists and LXD is healthy.
5. Check the backend and LXD logs on the host.

Creating a project requires metadata, durable directories, an LXD launch, mounts, limits, and best-effort provisioning.

### Project is stopped, missing, or in error

Open **Project settings → Settings** and choose **Start project**. A missing container is recreated from the base image and the durable workspace/provider homes are reattached.

### Project is running but has no network

Open **Info** and choose **Repair network**. Confirm that the project receives a non-loopback IPv4 address. If it does not, inspect the LXD bridge, DNS integration, and the host's IPv4-heal service.

### Project repeatedly runs out of memory or CPU

1. Check current and peak usage in **Info**.
2. Stop unnecessary processes from **Terminal**.
3. Ask an admin to adjust project limits under **Settings**.
4. Use **Force restart** if the container cannot recover internally.

Lowering memory can kill processes. Increasing one project's limits still consumes the single parent host.

### Disk is full even though root-disk usage looks acceptable

The optional root-disk quota does not cover the durable `/workspace` or provider-home bind mounts. Inspect the parent host filesystem and project directories. Remote has no default workspace quota or built-in backup cleanup.

### A package disappeared after an update

Only `/workspace` and provider homes survive container replacement. Reinstallable dependencies should be declared in the project and restored by its setup script; do not rely on ad-hoc packages installed elsewhere in the root filesystem.

## Agents and chats

### The provider is unavailable

Follow the provider's instructions under **Settings → Agents**. For a managed
flow, an admin can refresh its login and wait for the authenticated state. For
an external flow such as Antigravity, authenticate in the project Terminal and
then choose **Refresh models**. A no-auth module has no login action; verify its
CLI/configuration instead. Reopen or restart the project if credential
propagation is stale.

### Models or controls are missing or stale

Make sure the project is running, then open the provider/model picker and
choose **Refresh models**. This bypasses the
shared backend entry and probes the CLIs in the active project container, or on
the host for a loose chat. Use it after a CLI or configuration change, an
account-entitlement change, or a manual terminal login.

Healthy results are held in backend memory for 24 hours; a result containing
any provider fallback or warning is held for 2 hours. Expiry is checked on the
next request, and restarting the backend clears the cache. The browser keeps
the previous catalog visible while refresh runs. Provider-level fallback and
partial-discovery warnings are not generally rendered in the picker, so if
choices remain absent or generic, inspect the API response or backend logs and
verify the provider CLI directly.

### A run does not start

Check:

1. The workspace socket and chat socket are connected.
2. The project is running.
3. The selected provider's declared authentication/access requirement is satisfied.
4. No other client already owns the one-run-per-chat lock.
5. The chosen model, reasoning, or speed value is accepted by that provider.

### The agent is taking the wrong kind of action

Modes are advisory prompt policies, not sandbox profiles. Put important constraints and stopping conditions in the prompt. Use separate projects for stronger boundaries, and supervise external or destructive work directly.

### My queued prompt disappeared

Queued prompts and drafts use `sessionStorage` in the current browser tab.
They survive chat switching, app navigation, and a reload in that tab, but are
not shared with another tab, browser, device, or user. Closing the tab ends
their intended lifetime. A background chat's queue does not dispatch until you
open that chat again.

If delivery was rejected before the server accepted the prompt, Remote keeps it
queued for the next send window. For work that must run with no browser tab
open, use [Scheduled tasks](09-scheduled-tasks.md).

### The conversation lost earlier context

Provider switches, rewinds, missing sessions, and some forks can start a fresh provider session. Remote then supplies only a bounded visible transcript, approximately 24 KiB. Restate crucial requirements or keep them in project files such as `README.md` or `AGENTS.md`.

### A running agent vanished during a deploy or restart

Runs execute under the current backend process and cannot be reattached after it restarts. Review the durable files and chat events, then start a new prompt. Verify partial changes before continuing.

### Kimi behaves differently from Claude or Codex

Kimi currently has no usage telemetry, its fork starts fresh, and it does not
receive the equivalent Browser MCP plumbing. Selected skills are injected as
instructions to read their canonical `SKILL.md` paths rather than as native
provider triggers.

### Antigravity says it is not signed in

Antigravity is authenticated per project. Its **Settings → Agents** card shows
instructions but cannot perform the external login. Open the project's
**Terminal**, run `agy`, complete the displayed URL-and-code
flow, exit the CLI, and choose **Refresh models** in the chat picker before
retrying the prompt. Its `/root/.gemini/antigravity-cli` state is durable
across container replacement.

Antigravity also differs from Claude and Codex: it streams plain text rather
than structured tool/usage events, the Browser skill is not wired, and a fork
starts fresh. Selected skills are injected as canonical `SKILL.md` instruction
paths, and Scheduled Tasks also receives its scoped capability.

## Attachments, files, terminal, and IDE

### An attachment fails or stalls

Confirm project access, free disk space, and the configured upload limit. The default maximum is 10 GiB. Uploads use resumable tus chunks, but finalization still needs space in the project `.uploads` directory.

### File search finds nothing

Search is filename substring search, not content search. Use at least two characters. Results cap at 300 after visiting at most 200,000 entries. Very large individual directory listings cap at 10,000 entries.

### Selecting a file downloads it instead of opening it

Archives and unsupported image, audio, or video formats deliberately fall back
to download. Supported image/audio/video/PDF files open in Remote's media
viewer; code, data, text, logs, and other non-media files open in the project
IDE. Use the explicit download icon when you want the local file regardless of
type.

### Folder download fails

Folder ZIP downloads are limited to 1 GiB and two simultaneous archives across the server. Symlinks that escape the project path are omitted. Use Git, Terminal, or external storage for a larger export.

### Terminal closed after a network interruption

The Terminal pane keeps its PTY when it is merely hidden and reopened in the
same loaded chat. Losing its socket, switching chats, reloading, or closing the
page kills that shell, and there is no reconnect. Run durable processes under
an appropriate project process manager or terminal multiplexer that you
configure inside the project.

### IDE opens the wrong place

Open **Open in IDE** from the intended project chat, or use an
agent-generated validated absolute workspace path. The default path is
`/workspace`. Links can include `:line` or `:line:column`; Remote validates the
path and passes a code-server workbench payload so the cursor opens at that
location.

### A user can open an IDE for a project they do not belong to

This is a known authorization gap: the IDE proxy currently checks registered-user status but not project membership. Remove the person from the global user directory if access must stop, and do not invite mutually untrusted users to the same server.

## Preview and browser

### Open Browser says “No running apps”

1. Start the development server inside the project.
2. Bind it to `0.0.0.0`, not `127.0.0.1`.
3. Use a port from 1024 through 65535.
4. Choose the refresh control in the Browser drawer.
5. Select the discovered process and port.

### The preview URL exists but does not load

Check project membership, project IPv4 state, Caddy, DNS, and the process listener. The first request for a new project/port hostname may wait for on-demand TLS issuance.

### Inspect mode does not select anything

Reload the preview, confirm the app is loading on the expected project preview origin, then toggle the crosshair again. Agent Browser mode and app-inspection mode are mutually exclusive.

### Agent Browser stays on starting

1. Confirm the project is running.
2. Wait for Chromium, CDP, and noVNC provisioning.
3. Reload the pane.
4. Stop the Agent Browser explicitly and start it again.
5. Check container capacity and browser-service logs.

Closing the drawer stops only the human view. The browser core can stay running until explicit stop or the idle reaper.

### The website asks for consent or blocks automation

Take control in the shared browser, complete the permitted human step, then let the agent continue. Some sites prohibit automation; follow the site's terms and never bypass an access-control challenge.

## Git history

### History says “No git repos”

Confirm that a Git repository exists under `/workspace`. Discovery is bounded to depth 6 and skips common generated or heavy directories. A deeper repository may not appear.

### The diff is incomplete

Commit diffs are capped at 768 KiB. Open the repository in the IDE or Terminal for the full diff.

### Switch does not complete on a dirty repository

The backend supports creating a safety checkpoint before checkout, but the current History drawer does not render the checkpoint form needed to submit it. Open **Terminal**, commit or stash the dirty work, refresh **History**, and choose **Switch** again.

## Scheduled tasks

### The agent created a task but it never runs

Agent-created tasks start paused by design. Open **Schedules** in the same
project chat, review the definition, and select **Arm**. The agent cannot arm
or resume a task on your behalf.

### A recurring task is rejected as too frequent

The default minimum interval is five minutes. Widen the five-field cron
expression. An operator can change `SCHEDULE_MIN_INTERVAL`, but lowering the
guardrail increases unattended workload.

### A task says queued or skipped

Only one run can use a chat at a time. The default overlap policy coalesces
missed occurrences into one pending follow-up. A task configured to skip
records and consumes the busy occurrence instead. Open the chat, let the
current run finish, and refresh **Schedules**.

### A task is completed, exhausted, or in error and cannot resume

Those are terminal states. `completed` means its standing goal was marked
done; `exhausted` means it reached `maxRuns`; `error` usually means the owner,
chat, or project authorization is no longer valid. Fix the underlying access
or definition problem and create a new task.

### I switched successfully but cannot see a branch

History restore checks out the selected commit in detached HEAD. Create or switch to a branch in Terminal or IDE before making work you intend to retain.

## Secrets and sharing

### A new secret is not visible to an app

Restart the app process. Existing processes retain the old environment. If the value is multiline, read it from `/workspace/.env` or the agent-run environment; multiline values are not placed into LXD `environment.KEY`.

### A deleted secret still appears

Secret propagation is best-effort. Check the authoritative UI, `/workspace/.env`, LXD config, and any still-running process. Stop the process, remove the stale copy with host/operator care, and rotate the credential if exposure matters.

### I cannot add someone to a project

An admin must register the exact email under **Settings → Users** first. Then a current project member can add it under **Project settings → Sharing**.

## Last-resort operator checks

If the browser controls cannot recover the system, an administrator with host access should inspect:

- `systemctl status remote.futrx`;
- `journalctl -u remote.futrx`;
- Caddy status and logs;
- `lxc list`, container state, and LXD bridge health;
- parent-host disk and memory pressure;
- the documented health endpoint and startup reconciliation.

See [Deployment and operations](../04-operations/09-deployment-and-operations.md) for commands and [Known limitations](../known-limitations.md) before assuming a missing recovery feature exists.
