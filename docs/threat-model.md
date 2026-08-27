# Threat model

This document analyzes the security posture of remote.futrx: the trust boundaries it defends, the threats against each, what the code already does about them, and the residual gaps. It is written for operators deciding how to deploy the platform and for contributors changing security-relevant code. Read the [architecture](../ARCHITECTURE.md) first — this document assumes its vocabulary (host classes, layers, container model).

## Scope and method

The findings below come from a code review of the backend, frontend, and infra as of this writing. The ten highest-impact claims were **independently re-verified against the source** by reading the actual code paths; those are marked **✓ code-verified**. Others are cited to specific files but not separately re-verified.

Severity reflects impact **and** precondition. Note the platform's baseline trust assumptions, because they shape every rating:

- remote.futrx is **single-tenant-administrator by design**: one local admin, plus users the admin explicitly invites via Google. There is no self-signup, so "any registered user" means "any account the admin invited."
- Agents run **as root inside their container with safety rails off** (`--dangerously-skip-permissions` / `--dangerously-bypass-approvals-and-sandbox`). Running untrusted-influenced code is the product's *function*, so "code executes in a container" is the normal case, not an exotic precondition.
- The backend runs **as root on the host**.

These are deliberate design decisions, not bugs. The threats below are about what follows from them.

### Scheduled execution scope

Scheduled tasks extend project-agent authority beyond an open browser session,
so an armed task should be treated as unattended code execution with the same
project files, secrets, network access, and provider identity as an interactive
turn. The current design narrows that authority in several ways: agent-created
tasks start paused until a user arms them; interactive schedule grants are
fenced to one owner, chat, and project; scheduled fires receive only a
claim-bound `complete-self` grant and cannot create or modify schedules; and
the service applies recurrence, concurrency, and per-project task limits.
Operators should still use bounded `maxRuns`, narrowly scoped project secrets,
and the same egress and credential controls recommended below. See
[Scheduled tasks](02-workspaces/06-scheduled-tasks.md) for the complete state
machine and guardrails.

## Summary of findings

| # | Threat | Boundary | STRIDE | Severity | Verified |
| --- | --- | --- | --- | --- | --- |
| 1 | Loose (project-less) chat → root code execution on the host, reachable by any invited user | Agent / Web | Elevation of privilege | **Critical** | ✓ |
| 2 | Host tmux WebSocket / `/api/sessions` → host shell as root, no admin gate | Web | Elevation of privilege | **Critical** | ✓ (direct) |
| 3 | Operator-selected update ref executes as root without commit/tag signature verification | Supply chain | Elevation of privilege | **High** | cited |
| 4 | Any invited user reaches any project's code-server IDE (cross-tenant root shell) | Web | Elevation of privilege | **High** | ✓ |
| 5 | Cross-container lateral movement over the unsegmented LXD bridge | Container | Elevation of privilege | **High** | ✓ |
| 6 | Prompt-injected agent exfiltrates secrets over unrestricted egress | Agent | Information disclosure | **High** | cited |
| 7 | Provider OAuth tokens copied into every container, readable/exfiltrable by an injected agent | Secrets | Information disclosure | **High** | cited |
| 8 | Untrusted container poisons host-canonical provider tokens via post-run sync-back | Secrets | Tampering | **High** | ✓ |
| 9 | Agent Browser skill drives the user's live authenticated web sessions | Agent | Elevation of privilege | **High** | cited |
| 10 | Project members read plaintext secrets and can re-share the project | Secrets | Information disclosure | **High** | ✓ |
| 11 | Google OAuth authorizes on an unverified email claim | Web | Spoofing | **High**¹ | ✓ |
| 12 | Unpinned/unverified upstream fetches baked into host and every container as root | Supply chain | Tampering | **High** | cited |
| 13 | A single workspace can exhaust host disk (no disk quota) | Container | Denial of service | **High** | ✓ |
| 14 | Login rate limiter bypassed via `X-Forwarded-For` spoofing | Web | Spoofing | **Medium** | ✓ |
| 15 | `/ws/workspace` leaks other users' project/chat metadata | Web | Information disclosure | **Medium** | ✓ |
| 16 | Stateless 30-day sessions with no revocation | Web | Spoofing | **Medium** | ✓ |
| 17 | No CSRF tokens and WebSocket origin checks disabled | Web | Tampering | **Medium** | cited |
| 18 | Secrets/OAuth key plaintext at rest; leaked `session.key` forges admin sessions forever | Secrets | Elevation of privilege | **Medium** | cited |
| 19 | `return_to` open redirect into untrusted preview/IDE subdomains | Web | Spoofing | **Low** | cited |

¹ Conditional — see finding 11 for the precondition.

---

## Boundary 1 — Internet → Caddy → backend

External users reach only Caddy, which terminates TLS and forwards to the loopback backend. The backend trusts Caddy's `X-Forwarded-*` headers because nothing else can reach it. What's well-defended here: TLS is automatic; `/internal/*` is blocked externally; the session cookie is `HttpOnly; Secure; SameSite=Lax`; the SPA has a deliberately narrow XSS surface (vnode-only markdown, href allowlist, no `innerHTML`); on-demand TLS is gated so random subdomains cannot burn ACME quota. The gaps:

### 2. Host tmux WebSocket grants a root host shell to any invited user — **Critical** ✓

**Elevation of privilege.** `/ws?session=<name>` ([`transport/ws/tmux_socket.go`](../backend/internal/transport/ws/tmux_socket.go)) attaches a PTY to a host tmux session, and `/api/sessions` + `/api/sessions/{name}/send` ([`tmux_handler.go`](../backend/internal/transport/http/handlers/tmux_handler.go)) create sessions and inject keystrokes. These run on the **host** as the backend's service user — root. The only gate is the global middleware (registered user); there is **no admin or membership check**. Any invited member can open `/ws?session=x` and get an interactive root shell on the host.

- **Existing mitigations:** session names are validated (`^[a-zA-Z0-9_-]{1,32}$`); the main UI does not surface this route.
- **Residual gap:** no authorization beyond "registered." This is a full host compromise available to any non-admin invited user. Directly verified.

### 4. Any invited user reaches any project's code-server IDE — **High** ✓ code-verified

**Elevation of privilege.** The `forward_auth` handler ([`auth_verify_handler.go`](../backend/internal/transport/http/handlers/auth_verify_handler.go)) extracts the project slug **only** from the dev-preview host pattern `^([a-z0-9][a-z0-9-]*)--(\d{4,5})\.dev\.(.+)$`. For the IDE host classes — `<slug>.code.<host>` and `code.<host>/<slug>/` — the slug is never parsed, so [`access.go`](../backend/internal/service/auth/access.go) skips the membership branch and falls through to a **registered-user-only** check. code-server itself runs `auth: none` ([`code-server-up.sh`](../backend/internal/integration/containers/codeserver/assets/code-server-up.sh)) with an integrated root terminal over the bind-mounted project root. So any invited user can hand-craft `https://<victim-slug>.code.<host>/` and get a root shell and full read/write in a project they were never granted.

- **Existing mitigations:** Caddy `forward_auth` does require an authenticated, registered session, and strips platform cookies before proxying. The dev-preview URL path (`--<port>.dev`) *does* enforce membership — proving the mechanism exists and is simply not applied to the IDE host class.
- **Residual gap:** no per-project membership check for the IDE/code hosts. This is documented as a known gap in [`docs/02-workspaces/02-auth-users-and-access.md`](02-workspaces/02-auth-users-and-access.md), but the Caddyfile comments incorrectly call it "the same admin gate as the rest of the platform."

### 11. Google OAuth authorizes on an unverified email — **High** (conditional) ✓ code-verified

**Spoofing.** The Google userinfo response is decoded as `{id, email, name, picture}` — `verified_email` is never read ([`googleoauth/client.go`](../backend/internal/integration/googleoauth/client.go)) — and authorization is a bare normalized-email match against the invited-users list, never bound to the immutable Google `sub` or a hosted-domain (`hd`) ([`google_authenticator.go`](../backend/internal/service/auth/google_authenticator.go)). An attacker who can get Google to emit an invited user's email while unverified logs in as that user, inheriting their memberships (and admin role, if that invited user is a role-admin).

- **Precondition:** Google OAuth must be admin-enabled (it is off until configured), and the target email must be on a domain where an attacker can create an account with that address unverified — realistic for custom/corporate domains, **not** for `@gmail.com` (Google won't present a gmail address unverified). The local bootstrap admin is immune (blocked from the Google path).
- **Residual gap:** reject `verified_email == false`, bind to `sub` on first login, and/or enforce an `hd` allowlist. None of these exist.

### 14. Login rate limiter bypassed via `X-Forwarded-For` — **Medium** ✓ code-verified

**Spoofing.** The 5-failures-per-5-minutes limiter on `/auth/local/login` and `/auth/local/claim` keys on the **leftmost** `X-Forwarded-For` value ([`auth_local_handler.go`](../backend/internal/transport/http/handlers/auth_local_handler.go) `localClientIP`), and Caddy is not configured with `trusted_proxies`, so it appends the real client IP rather than replacing the header. Rotating the spoofed value per request defeats the lockout entirely.

- **Existing mitigations:** argon2id (m=19 MiB) makes each guess CPU-costly; uniform error responses resist enumeration; the admin password minimum is 12 chars.
- **Residual gap:** key the limiter on the trusted rightmost hop / `RemoteAddr`, or configure `trusted_proxies`. There is no account-wide throttle to fall back on.

### 15. `/ws/workspace` leaks non-member project/chat metadata — **Medium** ✓ code-verified

**Information disclosure.** [`workspace_socket.go`](../backend/internal/transport/ws/workspace_socket.go) filters the initial snapshot by membership for non-admins, but the live relay loop forwards **every** `project.upsert/delete` and `chat.upsert/delete` hub event to every subscriber with no per-event check. A non-admin passively harvests slugs, names, and lifecycle changes of projects they cannot access — and those slugs are exactly what finding 4 needs to target a victim.

- **Residual gap:** apply the membership predicate per event, not only to the snapshot.

### 16. Stateless 30-day sessions, no revocation — **Medium** ✓ code-verified

**Spoofing.** Sessions are stateless HMAC tokens with a hardcoded 30-day expiry ([`session_codec.go`](../backend/internal/service/auth/session_codec.go)); there is no server-side store. Logout only clears the cookie. A captured token is replayable for up to 30 days. The only kill switches are deleting the user (a per-request `IsRegistered` check then rejects them) or rotating `session.key` (which logs everyone out).

- **Residual gap:** no per-session revocation, no idle timeout, no rotation-on-logout. Severity rises to High if any token-leak vector exists.

### 17. No CSRF tokens; WebSocket origin checks disabled — **Medium**

**Tampering.** No state-changing route carries a CSRF token, and the WS upgrader's `CheckOrigin` unconditionally returns true ([`http/websocket.go`](../backend/internal/transport/http/websocket.go)), including for the host-shell and container-shell sockets. Protection rests entirely on the `SameSite=Lax` cookie and the same-origin edge.

- **Existing mitigations:** `SameSite=Lax` does block script-initiated cross-site WS handshakes and POSTs in current browsers, so this is not exploitable from an unrelated origin today. OAuth uses a random state cookie.
- **Residual gap:** protection is one cookie attribute deep with no defense-in-depth. Any regression to `SameSite=None`, or a content-injection foothold on a sibling subdomain, directly exposes host/container shells.

### 19. `return_to` open redirect into preview/IDE subdomains — **Low**

**Spoofing.** `isSafeReturnTo` ([`auth_redirect.go`](../backend/internal/transport/http/handlers/auth_redirect.go)) accepts any HTTPS URL on the base host **or any subdomain** — including `*.dev.<host>` and `*.code.<host>`, which serve untrusted container content. A crafted `?return_to=` can bounce a freshly authenticated user onto attacker-influenced content on a trusted-looking origin (e.g. a fake password prompt).

- **Existing mitigations:** external domains are rejected (https + base-or-subdomain only, ≤2048 chars); the target subdomain is itself `forward_auth`-gated.
- **Residual gap:** restrict post-login redirects to the main app origin.

---

## Boundary 2 — Registered user → the AI agent (confused deputy)

The agent runs as root with approvals disabled and ingests untrusted content — web pages via the Agent Browser, project files, chat attachments, tool output. Any of these can carry injected instructions. This is the highest-leverage boundary because the agent is powerful by design.

### 1. Loose chat → root code execution on the host — **Critical** ✓ code-verified

**Elevation of privilege.** A chat with an empty `ProjectID` runs the agent CLI **directly on the host**, not in a container ([`integration/agents/claude/command.go`](../backend/internal/integration/agents/claude/command.go), [`integration/agents/codex/command.go`](../backend/internal/integration/agents/codex/command.go): the `req.ProjectID == ""` branch execs `claude`/`codex` on the host). The backend is root, and the CLI carries `--dangerously-skip-permissions` + `IS_SANDBOX=1` (which explicitly permits uid 0). Any invited user can create a loose chat — the chat WebSocket only checks membership when `ProjectID != ""` ([`chat_socket.go`](../backend/internal/transport/ws/chat_socket.go), [`service/chat/access.go`](../backend/internal/service/chat/access.go)) — and either drive it directly or feed it injected content. Either way the result is arbitrary root code execution on the host, exposing `session.key`, `oauth.json`, the admin hash, every project's secrets, and all provider tokens.

- **Existing mitigations:** the global middleware requires a registered session; Codex strips `OPENAI_API_KEY` and pins `CODEX_HOME`.
- **Residual gap:** there is no container boundary on loose chats, no tool allowlist, and no membership gate on creating/running them. Consider running loose chats in a throwaway container, or restricting them to admins.

### 6. Injected agent exfiltrates secrets over unrestricted egress — **High**

**Information disclosure.** Project secrets are injected into the agent's environment (`lxc exec --env KEY=VALUE`) and mirrored to `/workspace/.env` ([`env_writer.go`](../backend/internal/service/project/env_writer.go)), and the bundled `AGENTS.md` tells the agent the network is fully open and even how to enumerate tokens. There is no egress filtering ([`resources/manager.go`](../backend/internal/integration/containers/resources/manager.go) adds none) and no data-loss guardrail. Injected content (web page, project file, attachment) can steer the agent to read `GITHUB_SSH_KEY`, cloud tokens, etc. and POST them to an attacker endpoint.

- **Existing mitigations:** secret files are 0600 on the host; Codex drops `OPENAI_API_KEY` from container env; multiline values are kept out of persistent LXD env.
- **Residual gap:** no outbound allowlist, no exfiltration guardrail. The only "policy" is prose in the browser skill, and it covers browser writes only.

### 9. Agent Browser drives the user's authenticated web sessions — **High**

**Elevation of privilege.** When the `browser` skill is enabled, `@playwright/mcp` attaches to the **same** Chromium the user logs into by hand ([`browser/mcp.go`](../backend/internal/integration/containers/browser/mcp.go), CDP `127.0.0.1:9222`). The agent inherits the user's cookies for whatever they signed into, and its perception loop reads page content — so a visited page can inject instructions to post, DM, buy, change settings, or read and exfiltrate private data on the user's behalf.

- **Existing mitigations:** the MCP is wired only when the skill is selected; CDP and RFB are loopback-only inside the container; the skill prose asks for confirmation before writes.
- **Residual gap:** no technical enforcement of write-approval, no URL allowlist, unrestricted reads of authenticated content. Classic confused deputy over a live session.

### Related: default runs and skill instructions are approval-free

Remote now forwards only provider-native Default and Plan modes; it does not
prepend custom workflow prompts. Default runs still bypass provider approvals
inside the project container, and Codex Plan is implemented by provider-owned
collaboration instructions rather than an OS-level read-only sandbox. There is
no Remote human-confirmation gate for irreversible or external actions. See
[Known limitations](known-limitations.md).

---

## Boundary 3 — Container → host and container → sibling container

Unprivileged LXC namespaces plus the managed resource profile are the isolation boundary. The soft spots are the shared network bridge and the missing disk quota.

### 5. Cross-container lateral movement over the LXD bridge — **High** ✓ code-verified

**Elevation of privilege.** All project containers share one unsegmented `lxdbr0` with no `security.mac_filtering` and no per-container firewall; the host UFW opens only 80/443, which does not filter peer-to-peer traffic. Each container exposes root-level services reachable on the bridge: code-server on `0.0.0.0:8842` (`auth: none`) and noVNC/websockify on `0.0.0.0:6080` (`x11vnc -nopw`). Code executing in container A can connect to `<B>.lxd:8842` — a root IDE/terminal in B — or `<B>.lxd:6080` — B's live authenticated browser — completely bypassing Caddy's edge auth.

- **Existing mitigations:** code-server binds loopback behind the `:8842` socket-activation proxy; CDP and raw RFB are loopback-only; Caddy gates the *edge*.
- **Residual gap:** nothing gates the direct bridge path. Precondition is only code execution in one container — the platform's core function — so prompt-injection-to-lateral-movement is a first-class path. Fix with LXD network ACLs or per-container nftables default-deny for peer ingress on 8842/6080/9222/5900, or bind those services host-only.
- *Correction to earlier analysis:* `ipv4.firewall` **is** enabled by `lxd init --auto`, but it governs host-bridge NAT/DHCP, not inter-container isolation, so the conclusion stands.

### 13. A single workspace can exhaust host disk — **High** ✓ code-verified

**Denial of service.** The managed profile caps memory/CPU/processes but sets **no disk quota** ([`resources/manager.go`](../backend/internal/integration/containers/resources/manager.go)). `/workspace` and the agent homes are **host bind mounts** under `/var/lib/remote/projects`, so writes go straight to the host filesystem — and LXD `size` quotas never apply to bind-mounted source devices. The default `dir` storage backend (`lxd init --auto`, no `--storage-backend`) doesn't support size quotas anyway. Any workspace running `dd`/`fallocate`/a runaway build fills the host root filesystem shared by LXD, the backend, `DATA_DIR`, and sshd — a host-wide DoS.

- **Existing mitigations:** memory/CPU/process caps (added after two real host takedowns) are enforced per cgroup; an admin-only per-project `root` disk `size` override exists but applies to the container overlay, not the bind mount.
- **Residual gap:** no disk ceiling by default. This is the exact gap the CPU/memory remediation left open. Fix with a project quota (XFS/btrfs) on the bind-mount source or a quota-capable storage pool with a default size.

### Related: `security.nesting=true` fleet-wide

Every container gets `security.nesting=true` ([`resources/manager.go`](../backend/internal/integration/containers/resources/manager.go)), which widens the kernel attack surface for a container-escape attempt. Chromium currently starts with `--no-sandbox`, so nesting is not providing a Chromium sandbox. Containers are unprivileged (no `security.privileged` anywhere), which is the main mitigation. Also note resource-cap convergence on the start path swallows its error (`_ = s.resources.Ensure(...)`), so a container can run without the intended default profile limits until a later successful reconcile.

---

## Boundary 4 — Credential lifecycle

Provider tokens, project secrets, the session key, the OAuth client secret, and the admin hash all live unencrypted under root's home / `DATA_DIR`. File mode 0600 keeps out non-root host users but is **no barrier to the root-uid agent or backend**, which own the files.

### 7. Provider tokens copied into every container, readable by an injected agent — **High**

**Information disclosure.** Host-canonical provider OAuth tokens (`/root/.claude*`, `/root/.codex/auth.json`, `/root/.kimi-code/*`) are pushed into every project container before a run and persist in the per-project agent-home bind mount ([`integration/agents/claude/profile.go`](../backend/internal/integration/agents/claude/profile.go), [`credentials/files.go`](../backend/internal/integration/containers/credentials/files.go)). The agent runs as root, so mode 600 does not protect them from it. An injection that makes the agent read and exfiltrate `.credentials.json` hands the attacker the operator's Anthropic/OpenAI/Kimi **subscription** tokens, reusable off-box until rotation. The same tokens sit in every container (finding 4/5 make them reachable cross-tenant too).

- **Existing mitigations:** unprivileged containers; Codex refuses api-key mode (subscription-only); the backend never handles provider passwords.
- **Residual gap:** one shared platform identity copied into every untrusted workspace; no per-project scoping, no short-lived tokens.

### 8. Untrusted container poisons host-canonical tokens via sync-back — **High** ✓ code-verified

**Tampering.** After a successful run (`err == nil`), the host **pulls** credential files the container wrote back onto the canonical host paths, chmods 0600, and stamps `mtime` to now ([`provider.go`](../backend/internal/integration/agents/claude/provider.go), [`credentials/files.go`](../backend/internal/integration/containers/credentials/files.go)). The pull is **unconditional** (no mtime comparison), so a compromised agent that overwrites its own `/root/.claude/.credentials.json` and exits 0 gets its tokens adopted as canonical. Because the host copy is then stamped newer, the next `pushIfNewer` re-pushes the poisoned tokens into **every other container**. No signature, provenance, or content validation is applied to pulled credentials.

- **Existing mitigations:** sync-back only fires on a clean exit; directory sync uses basenames only (no path traversal); Codex re-checks auth mode after sync.
- **Residual gap:** "successful run" is fully attacker-controlled, and the host trusts whatever the container writes. Fleet-wide credential poisoning / auth DoS from a single container.

### 10. Members read plaintext secrets and can re-share — **High** ✓ code-verified

**Information disclosure.** Any project **member** — not just admins — can `GET /api/projects/{id}/secrets` and receive plaintext values, `PUT`/`DELETE` secrets, and add other registered users to the project ([`project_handler.go`](../backend/internal/transport/http/handlers/project_handler.go)). Only `limits` and project-delete are admin-gated. Sharing a project therefore shares its secrets and the ability to re-share.

- **Existing mitigations:** members can only add already-registered users; a non-admin cannot remove the last member; keys are validated for shell safety.
- **Residual gap:** this is the platform's flat admin/member model (no owner tier, no per-secret ACL) rather than a bypass — but it means one curious or compromised member account exfiltrates every project secret. Secrets are also plaintext at rest and mirrored to `.env` inside the workspace.

### 18. Plaintext secrets at rest; leaked `session.key` = permanent admin forgery — **Medium**

**Elevation of privilege / Information disclosure.** `session.key`, `oauth.json` (Google client secret), `local-admin.json` (argon2id hash), and all `projectsecrets/*.json` are plaintext files. There is no encryption at rest and no key rotation mechanism. Anyone who obtains `session.key` (via findings 1/2, a backup, or a disk image) can mint a valid admin session **forever**; the only remediation is manually replacing the key (which logs everyone out). Provider CLI stderr and raw provider JSON are also persisted/logged unredacted, a potential credential sink if a CLI ever echoes a token.

- **Residual gap:** no at-rest encryption, no rotation/revocation, no secret scrubbing in logs. Treat `DATA_DIR` and root's home as crown jewels; back them up encrypted; restrict host access tightly.

---

## Boundary 5 — Host → upstreams (supply chain and operations)

The installer, updater, and base-image build pull code or packages as root from
GitHub and external registries. Release CI runs separately on GitHub-hosted
runners and does not deploy production.

### 3. An unsigned selected update ref executes as root — **High**

**Elevation of privilege.** Production is not deployed automatically from a
push. An operator explicitly invokes the in-app updater or `infra/update.sh`,
but that root-run updater fetches and hard-resets to a requested tag/ref (or
`origin/main`) and re-executes the selected script without
**`git verify-commit` / `verify-tag` / `merge.verifySignatures`**. A malicious
selected commit or tag can therefore own every container, provider token, and
project secret once an operator applies it.

- **Existing mitigations:** the update requires an explicit operator action;
  semantic-version tags pass the repository release classifier before the
  release workflow publishes them; Go dependencies are checksum-verified via
  `go.sum`.
- **Residual gap:** neither the installer nor updater requires signed commits
  or tags. Branch protection and release approval policy, if configured, live
  outside the repository. GitHub Actions used for release/vendor publication
  are pinned to mutable major tags such as `@v4`, although those workflows do
  not directly deploy production.

### 12. Unpinned/unverified upstream fetches baked in as root — **High**

**Tampering.** Several root-context fetches have no cryptographic integrity check: NodeSource `curl | bash` (major-version pinned only), the Go tarball (validated only by `tar -tzf` + the reported version string), the code-server `.deb` from GitHub releases (no checksum), `snap install lxd` (unpinned), and the `ubuntu:24.04` base image (floating). All of this is rebuilt on every update and re-run inside live containers via the CLI repair path. A compromise or MITM of any of these upstreams runs as root and propagates into every workspace on the next rebake.

- **Existing mitigations:** Caddy and GitHub CLI apt repos are GPG-signed; the Go install stages with backup/rollback; LXD's `ubuntu:` remote verifies image signatures; agent CLI and host versions are centrally pinned in one canonical manifest (`infra/versions.env` is a symlink).
- **Residual gap:** no SHA256/signature pinning on the fetches above; the base image is non-reproducible. Also note the npm agent CLIs are pinned by version but installed with lifecycle scripts enabled and no integrity hashes — a backdoored publish *at* the pinned version, or a poisoned pin commit, runs as root host-wide.

### Also in this boundary

- A `--github-token` PAT is embedded in `/opt/remote.futrx/.git/config` (0600, but root — and every agent runs as root); the Google client secret can transit `argv`/shell history when passed as an install flag.
- Production frontend builds use `npm install`, not `npm ci`, so the committed lockfile is not strictly enforced at deploy.
- The full infrastructure updater mutates the checkout, host, base image, and
  eligible workspaces incrementally and has no transaction-wide rollback if a
  later convergence step fails. The patch-only application deployer does stage
  its binary and restore the previous checkout/binary on restart or health-check
  failure.

---

## Prioritized recommendations

Roughly in order of risk reduction per unit effort:

1. **Gate the host-shell and loose-chat paths** (findings 1, 2). At minimum require admin for `/ws?session=`, `/api/sessions/*`, and loose-chat runs; better, run loose chats in a disposable container. These are any-invited-user → host-root.
2. **Enforce project membership on the IDE host class** (finding 4) — parse the slug from `<slug>.code.<host>` and `code.<host>/<slug>/` and apply `HasAccess`.
3. **Segment the container bridge** (finding 5) — LXD network ACLs or per-container nftables default-deny on peer ingress to 8842/6080/9222/5900.
4. **Add a default disk quota** (finding 13) — move workspaces to a quota-capable pool or apply a project quota on the bind-mount source.
5. **Sign the update chain** (finding 3) — require verified signed commits/tags
   before the root updater re-executes selected code; preserve an explicit
   operator approval; SHA-pin GitHub Actions used for release publication.
6. **Validate credential sync-back** (finding 8) and scope provider tokens per project if feasible (findings 7, 8).
7. **Harden auth details:** check `verified_email` + bind to `sub` (11); fix the rate-limiter IP source / set `trusted_proxies` (14); filter `/ws/workspace` events (15); add per-session revocation (16).
8. **Constrain the confused-deputy surface:** egress allowlist and/or secret-access policy for agents (6), enforced write-approval and URL allowlist for the Agent Browser (9).
9. **Treat `DATA_DIR` + root's home as secrets at rest** — encrypted backups, tight host access, and a plan for `session.key` rotation (18).

## What this model does not cover

This is a design/code review, not a penetration test or a formal audit. It does not include dynamic testing, fuzzing, dependency-CVE scanning, or review of the LXD/Caddy/kernel/agent-CLI code the platform depends on. The absence of a finding here is not a guarantee of safety. Report anything you discover per [SECURITY.md](../SECURITY.md).
