# Known limitations

These are the constraints worth understanding before you deploy or rely on remote.futrx. They are current, deliberate consequences of the design — not a bug list. Security-specific limitations are analyzed in depth in the [threat model](threat-model.md); this document covers operational, scaling, and functional limits. Everything here is drawn from the code as it stands.

## Scale and availability

- **Single server, single process.** The backend is one systemd unit bound to `127.0.0.1:7682` on the same host as Caddy and LXD. All state lives in local files guarded by in-process mutexes and in-memory indexes, so a second backend instance — even on the same box — would corrupt or diverge state. There is no clustering, load balancing, failover, or multi-node support. If the server dies, everything (app, workspaces, previews, IDEs) is down. *(`infra/templates/remote.futrx.service.tmpl`, `backend/internal/stores/`)*
- **Capacity is bounded by the one host.** Projects, chats, and exposed ports all consume resources on a single box. The "unlimited projects/chats/ports" framing is a server-bound statement, not a product guarantee.
- **No horizontal scaling of any runtime component.** The run hub, workspace hub, tus upload store, login limiter, and archive-download semaphore are all in-process. Slow WebSocket subscribers are dropped once their buffer fills.

## Data, durability, and backups

- **No database.** Every datastore is a flat JSON file or per-chat JSONL log under `DATA_DIR` (`/opt/remote.futrx/data`). JSON and metadata writes use temp-file + rename, while chat events append directly to JSONL with `O_APPEND`. Neither path uses `fsync` or file locking, so a crash, power loss, or stray second process can lose or corrupt state. *(`backend/internal/stores/`)*
- **No backup or restore mechanism.** Nothing snapshots, exports, or restores `DATA_DIR`, the workspaces at `/var/lib/remote/projects`, or the containers. The only recovery-adjacent feature is the per-project Git "safety checkpoint," which protects code the agent edits — not chats, users, secrets, sessions, or container state. Disaster recovery is entirely the operator's responsibility, and a hand-rolled backup taken while the service runs can capture a torn view. **Snapshot the host or copy `DATA_DIR` yourself, ideally with the service stopped.**
- **Chat logs grow without bound.** Per-chat `events.jsonl` files are append-only with no rotation or compaction, and `List()` re-reads every `meta.json` from disk — practical scale is modest project/chat counts.
- **No cross-store transactions.** Project metadata, membership, and secrets are three separate files that can go out of sync. Deleting a project does not cascade-delete chats that reference it; deleting a user does not sweep their project-access records.
- **Secrets are plaintext at rest.** Project secrets, the Google OAuth client secret, and the session signing key are unencrypted files (mode 0600). Encryption at rest is not provided. See the [threat model](threat-model.md).

## Platform and deployment

- **Ubuntu/Debian only.** The installer hard-exits on any other distro and assumes systemd, Caddy, root execution, public DNS pointing at the box, and outbound reachability to Let's Encrypt. LXD is normally installed through snap. Inside an unprivileged Proxmox LXC, the installer uses Debian's native LXD package because snap requires host SquashFS/AppArmor facilities that are not exposed by default. The outer container must still have nesting enabled and delegate UID/GID `1000000-1065535`; the installer refuses to replace Remote's unprivileged project containers with privileged ones when that range is missing. There is no air-gapped, Docker, or non-Linux deployment path. *(`infra/install.sh`, `infra/steps/01-host-deps.sh`)*
- **The backend runs as root.** With `KillMode=process`, a compromise of the app process is a compromise of the host. There is no privilege separation for the backend. *(`infra/templates/remote.futrx.service.tmpl`)*
- **The installer disables SSH password login.** Confirm your SSH key works before running it (this is called out in the README, and repeated here because it is easy to lock yourself out).
- **Infrastructure updates hard-reset the installed checkout.** `update.sh`
  selects the requested release tag/ref and defaults to `origin/main`, so any
  local source changes are discarded. It converges the host, application, base
  image, and workspaces incrementally, with no transaction-wide rollback if a
  later step fails. Patch-only `deploy-app.sh` is narrower and does restore its
  previous checkout/binary when build, restart, or health validation fails.

## Updates and workspace upgrades

- **Container upgrades are lossy outside `/workspace`.** Upgrading to a new base image re-clones the container: only `/workspace` and the agent homes (bind mounts) survive. Anything installed in the rootfs — ad-hoc `apt`/`npm` installs, caches, system config — is lost on every upgrade. *(`infra/upgrade-workspaces.sh`)*
- **The updater's active-run skip is currently unreliable, and the app goes down during recycling.** The intended default is to skip containers with a running agent, but the busy-process matcher does not match every provider command shape and can classify active work as idle. Coordinate a maintenance window or use `--skip-workspaces` while agents are active. The recycle step also stops the main backend service for its duration, so updates that recycle workspaces take the whole UI down. `--include-busy` explicitly permits disruptive recycling; it is not needed to trigger the current detector gap.
- **A plain `install.sh` re-run can leave a stale base image.** The base-image build is skipped when the alias already exists; only `update.sh` (which forces a rebuild) or the runtime self-heal converges containers to new agent-CLI pins.

## Resource management

- **Default per-container caps are fixed in the binary and applied best-effort.** The backend-managed LXD profile targets 4 GiB memory, 6 CPUs, and 2000 processes (added after two real host takedowns — an ffmpeg CPU peg and a Node OOM). Changing the fleet default means editing source and redeploying. Launch attempts to restore the profile, but default-convergence errors are swallowed; explicit project overrides fail launch when they cannot be applied. Heavier workloads need a per-project override. *(`backend/internal/integration/containers/resources/manager.go`)*
- **No default disk quota, and no cap on project count.** A single workspace can fill the host disk, and any invited user can create unlimited projects/containers (each `boot.autostart=true`, so a host reboot starts them all at once). The per-container caps bound one container, not the aggregate. See the [threat model](threat-model.md) for the DoS implications.
- **No rate limiting except on local login.** Chat/agent runs, project creation, terminal/IDE sessions, and uploads (up to 10 GiB) have no per-user request-rate or quota limit.
- **No audit logging and no metrics endpoint.** There is no record of who created/deleted projects, read or changed secrets, ran agents, or invited users (the code has a placeholder comment where caller identity is discarded). Forensics rely on generic journald output. There is no Prometheus/OpenTelemetry instrumentation.

## Authentication and access

- **One password account.** Only the first-claimed local admin uses a password; every other user must sign in through Google OAuth. Until an admin configures Google client credentials, the box is effectively single-user, and users without a Google account can never log in. *(`backend/internal/service/auth/`)*
- **No password reset flow.** Recovering a lost owner password requires manually deleting `local-admin.json` on the host.
- **TOTP 2FA, single-active-session enforcement, sign-in history, and recovery-code alerting exist but are opt-in per account, off by default, and each independently toggleable** from Settings → Security. An account that leaves all four off is unaffected by any of them: no server-side session lookup, no history file, no alert field, and its sessions remain the plain 30-day stateless tokens described below. There is still no admin-facing "force disable 2FA" affordance for a member who has lost both their authenticator and all recovery codes — the local admin's only lever today is manually deleting that account's `DATA_DIR/twofactor/<hash>.json` file on the host, mirroring the existing `local-admin.json` recovery path above. Note also that TOTP codes are single-use: signing in twice inside the same 30-second step requires waiting for the authenticator to roll to the next code.
- **Sessions remain stateless by default; single-session revocation is opt-in, not universal.** A session that never turned on "single active session" (the vast majority) still cannot be individually revoked — the only levers for it are deleting the user or rotating the global session key. Turning that preference on makes a new login on any device immediately invalidate the account's previous session.
- **Flat admin/member roles.** There is no per-project "owner" tier. Any project member can read and change that project's secrets and edit its membership. Sharing a project shares its secrets.
- **The per-project IDE is reachable by any invited user.** `<slug>.code.<host>` authenticates the registered user but does **not** check project membership, so any invited user can open any project's code editor. This is documented in [`docs/02-workspaces/02-auth-users-and-access.md`](02-workspaces/02-auth-users-and-access.md) and analyzed in the [threat model](threat-model.md).
- **No public/anonymous sharing.** Every preview URL sits behind the platform session, so showing a prototype to an outside stakeholder means inviting them as a user first.

## Agents

- **Agent modules are compiled in, not runtime plugins.** Each provider package
  owns a validated factory combining its adapter, auth, feature policy, and
  provisioning profile. A new integration still requires an explicit reviewed
  factory entry in `internal/config/agents.go`, followed by a rebuild and
  deployment.
  Remote does not load third-party provider modules from configuration or
  shared objects.

- **Claude, Codex, and Kimi identity is a shared host singleton.** Those
  credentials are authenticated once at host level and seeded into every
  container, so all users and projects share the same provider accounts and
  subscription quotas. There is no per-user or per-project identity for those
  providers, and each allows only one interactive login at a time.
- **Codex's API-key guard does not inspect newer project-local auth before a
  run.** Remote rejects a host `auth.json` explicitly marked `apikey` and clears
  `OPENAI_API_KEY`, but credential seeding does not overwrite a newer
  project-local record. That record can therefore drive a project run. A
  successful pull detects the API-key mode only afterward, and the sync error
  is logged without failing the completed run.
- **The supported Antigravity authentication flow is project-local.** Users
  run `agy` in the project Terminal. Its credential and conversation state
  under `/root/.gemini/antigravity-cli` is a durable provider mount and survives
  container replacement. It remains shared by everyone with access to that
  project. A loose chat can use operator-prepared host `agy` state, but Remote
  exposes no host Antigravity login UI and a loose-chat Terminal cannot create
  that state.
- **Run control does not survive a backend restart.** Agent runs are owned by in-process state around an `lxc exec` child. A backend restart loses the run lock, cancellation handle, and event-stream ownership. With the production unit's `KillMode=process`, the child may remain alive but orphaned rather than being killed. There is no server-side run persistence, reattachment, or restart recovery.
- **One backend run per chat; interactive queueing remains browser-owned.** A
  direct concurrent run request is rejected. Drafts and queued prompts are
  mirrored to per-tab `sessionStorage`, so they survive navigation and reloads
  in that tab, but not tab closure or another tab/browser/device. A background
  chat's queue sends only after that chat is opened again. Use scheduled tasks
  for host-owned future work.
- **Session recovery drops context.** When a provider session is missing (or you switch provider mid-chat), the chat is "recovered" by replaying at most the last ~24 KB of visible transcript as plain text into a fresh session — earlier context and all tool-call state are dropped.
- **Provider Plan modes differ.** Remote forwards provider-native Default and
  Plan modes instead of adding workflow prompts. Claude Plan is read-only;
  Codex Plan is a provider collaboration-instruction preset rather than an
  OS-level read-only sandbox. Default project runs bypass provider approvals,
  and Remote has no human-confirmation gate for irreversible or external
  actions.
- **Provider-specific gaps.** Kimi has no fork primitive (forked Kimi chats
  silently start fresh) and reports no usage data. Its discovered per-model
  Thinking choice is displayed and saved but is not forwarded to the Kimi run,
  and the currently pinned Kimi CLI rejects its advertised Plan flag with the
  prompt mode Remote requires. Antigravity forks also
  start fresh; print mode exposes plain streamed text rather than structured
  tool/usage events, selected skills use explicit `SKILL.md` instruction paths
  rather than native triggers, and Browser MCP is unavailable. Model catalogs
  reflect the installed CLI, its configuration,
  the signed-in account, and current entitlements; they are not a promise that
  every provider model in existence is available to that account. Claude Fast
  mode requires an eligible Opus model, usage credits, and provider/account
  enablement. Failed Claude tool calls are currently rendered as successes.
- **Capability catalogs can lag external changes.** A fully live catalog is
  cached in backend process memory for 24 hours; any fallback or warning uses a
  2-hour TTL. CLI, configuration, and entitlement changes do not directly
  invalidate it. Managed authenticated-state changes and completed-login
  revisions detected by the browser, plus the sidebar's project **Start**
  action, request selected refreshes; intermediate login-status changes, the
  Project workspaces Start/Restart actions, and terminal-based changes such as
  Antigravity login require **Refresh models**. Restarting the backend clears
  the entire cache. Provider-level fallback and
  partial-discovery warnings are present in the API but, except for the
  Antigravity sign-in disable reason, are not currently rendered in the
  composer.

## Scheduled tasks

- **Repeated runs grow one provider session.** A scheduled task resumes the
  same chat/provider session, so long-lived recurrence accumulates context and
  token cost. Use `maxRuns`, complete bounded monitors, and periodically create
  a fresh task/chat.
- **Missed occurrences are coalesced, not replayed.** After downtime or a busy
  chat, Remote runs at most one overdue follow-up under the default overlap
  policy. It is not a durable event-processing queue with exactly-once replay.
- **Creation currently starts through the agent.** The drawer can arm, edit,
  pause, resume, run, and delete tasks, but it has no direct create form.
  Select the Scheduled Tasks skill and explicitly ask the agent to create the
  parked definition.
- **The scheduler is still single-process and file-backed.** Claims survive in
  `scheduled-tasks/tasks.json`, but timer ownership, concurrency accounting,
  and execution live in the one backend process. There is no distributed
  scheduler or external queue.

## Previews, IDE, and the Agent Browser

- **Preview ports must be 4–5 digits.** The dev-URL scheme only routes ports 1024–65535 (`<slug>--<port>.dev.<host>`). A server on port 80/443/999 inside the container cannot be exposed without rebinding higher.
- **Loopback-bound apps can't be previewed.** Port discovery deliberately excludes `127.0.0.1` binds, because Caddy proxies to the container's bridge address. Bind to `0.0.0.0` to get a preview URL.
- **On-demand TLS means first-hit latency and CA dependency.** Each new project/port triggers an individual Let's Encrypt certificate at first visit, with a hard dependency on the CA being reachable and exposure to per-domain rate limits as projects × ports grow. There is no DNS-challenge wildcard option.
- **One shared browser session per project.** The user and the agent drive the same Chromium profile and window (fixed 1366×768) — there is no per-agent or per-task browser isolation. Idle reaping counts any TCP connection to the VNC port as a viewer.
- **Launch provisioning is best-effort.** Credential seeding, skill links, browser tooling, and code-server setup all swallow their errors, so a container can come up with a broken IDE, missing credentials, or no browser tooling with nothing surfaced until you try to use it.

## Workspace tools and files

- **Directory and search limits are fixed constants.** Listings truncate at 10,000 entries per directory; search is filename-substring only (no content search), returns at most 300 results, and gives up after visiting 200,000 entries. In-browser preview is limited to an image/audio/video/PDF allowlist; other types can't be opened inline.
- **Archive downloads are capped.** Folder-ZIP downloads are limited to 1 GiB each with at most 2 concurrent downloads box-wide; a larger workspace simply fails.
- **Escaping symlinks are silently omitted** from listings and downloads (a safety measure, but it can hide files).
- **Git history detection is bounded.** Repository discovery skips a fixed directory blocklist at depth ≤ 6, so deeper or oddly-placed repos are invisible to the history UI. Commit lists cap at 200, diffs truncate at 768 KiB.
- **The dirty-tree safety-checkpoint UI is incomplete.** The backend checkout route can stage all changes, create a `remote.futrx` checkpoint commit, and switch to a detached commit, but the current History drawer does not render the form that submits the checkpoint message. Clean the tree through Terminal or IDE before using **Switch**.

## Frontend

- **No URL routing.** The active chat, current view, and open drawers are all in-memory state — a page refresh loses your selection, and nothing is deep-linkable.
- **The PWA is not an offline workspace.** The main app is installable and can
  receive Web Push notifications, but navigation remains network-first. Its
  service worker caches only a self-contained offline status page, not the app
  shell, chats, API data, or project content. Work cannot continue without a
  connection. The IDE launcher at `code.<host>` is a separate PWA.
- **The terminal has no reconnect logic.** A network blip ends the terminal view (unlike the chat/workspace sockets, which reconnect).
- **Automated tests cover only pure state modules.** Hooks, transport, API clients, and UI components are untested by the frontend test suite; CI does not run the Go `go test` suite either (run it locally — see [CONTRIBUTING.md](../CONTRIBUTING.md)).
