# RBAC dependent-stack execution report

Date: 2026-10-30 (Africa/Cairo). Repository: futrx-com/remote.futrx.

## Working state and integration order

Started on clean `feat/permission-abstraction` at `a8eb4dbc`.
Verified origin = `https://github.com/AbdallahMohamedDotnet/remote.futrx.git`,
upstream = `https://github.com/futrx-com/remote.futrx.git`.
Repository-local author and committer: Samuel Emad <samuelemad49@gmail.com>.
Fetched qa and all eight requested PR heads once. Stayed on
`review/rbac-dependency-reconciliation` after creating it from qa `07462337`.

Order: current #223 foundation → #315 canonical actor decisions/creation and
separate frontend responsibilities → #317/#324 adapted current workspace
boundaries → #320 account authorization → latest #44 audit plus selective #325
correlation/filtering → #325 final actor-fixture correction and integrated
security regression tests. #333 was a selective reference only.

## Findings, corrections and evidence

### FIX-01 — Competing policy API and creation gates

- **Finding/proof:** [`d23457bb:backend/internal/transport/http/handlers/permissions_handler.go:22`](https://github.com/futrx-com/remote.futrx/blob/d23457bb6ba5f49d9c2755fa4f866d62f18337ca/backend/internal/transport/http/handlers/permissions_handler.go#L22) versus [`a8eb4dbc:backend/internal/transport/http/handlers/permissions_handler.go:47`](https://github.com/futrx-com/remote.futrx/blob/a8eb4dbcab04e8a76100306708c17c12f996e96a/backend/internal/transport/http/handlers/permissions_handler.go#L47)
- **Validation:** Confirmed integration incompatibility.
- **Modification:** `rbac/queries.go: Effective, Policy`; canonical `permissions_handler.go: HandleEffective`; `service/project/permissions.go, service.go: Create`; `service/chat/permissions.go, service.go: Create, Fork`; `main.go` registry and `services.go` injection.
- **Invariant/reason:** Keep one registry, evaluator, policy store and administration contract. Creation and fork checks precede repository/container/event writes.
- **Tests/results:** Real HTTP policy CRUD, role binding/removal, encoded actor decisions and scope isolation; real HTTP authorized/denied project/chat creation and forks with persisted record-count assertions; service denial and delegation tests passed.
- **Status:** Fixed.

### FIX-02 — Hook signature and JSON-contract collisions

- **Finding/proof:** [`d23457bb:frontend/src/state/hooks/permissions/usePermissions.ts:6`](https://github.com/futrx-com/remote.futrx/blob/d23457bb6ba5f49d9c2755fa4f866d62f18337ca/frontend/src/state/hooks/permissions/usePermissions.ts#L6) versus [`a8eb4dbc:frontend/src/state/hooks/permissions/usePermissions.ts:33`](https://github.com/futrx-com/remote.futrx/blob/a8eb4dbcab04e8a76100306708c17c12f996e96a/frontend/src/state/hooks/permissions/usePermissions.ts#L33)
- **Validation:** Confirmed integration incompatibility; the older uppercase schema was not adopted.
- **Modification:** `usePermissionAdministration.ts`, `useEffectivePermissions.ts`, canonical `api/permissions.ts`, settings imports, sidebar creation hints, workspace/project controls, `models/rbac.ts`, scope forms and `permissions.contract.md`.
- **Invariant/reason:** Preserve the newer administration UI and lowercase serialized fields. Actor decisions refresh on focus, policy edits and interval, reject stale-scope responses and fail closed on request errors.
- **Tests/results:** 584 frontend tests and production build passed. Real Go HTTP encoding tests verify tagged fields, empty arrays, mutations and actor decisions; provider-account form validation passed. Live browser interaction testing was not performed.
- **Status:** Fixed; live UI smoke test remains unverified.

### FIX-03 — Older lifecycle snapshots omit current setup

- **Finding/proof:** [`a8eb4dbc:backend/internal/service/project/service.go:70`](https://github.com/futrx-com/remote.futrx/blob/a8eb4dbcab04e8a76100306708c17c12f996e96a/backend/internal/service/project/service.go#L70) and [`a8eb4dbc:backend/internal/service/project/service.go:476`](https://github.com/futrx-com/remote.futrx/blob/a8eb4dbcab04e8a76100306708c17c12f996e96a/backend/internal/service/project/service.go#L476)
- **Validation:** Confirmed incompatibility in stale dependent snapshots; current foundation already contains the required setup implementation.
- **Modification:** `service/project/service.go`: add creation/control/secret/browser gates around current operations and compose audit wrappers; adapt `service_create_test.go` and `service_setup_test.go` to explicit actors.
- **Invariant/reason:** Retain ContainerSetup wiring, secrets/environment propagation, start/restart/upgrade/recreation paths, locking, persisted setup failure and retry. Denied operations stop before domain side effects.
- **Tests/results:** Project lifecycle/setup/permission tests and targeted race checks passed. The separate unchanged Linux guest-wrapper integration tests fail locally because GNU timeout is absent.
- **Status:** Fixed; local guest-wrapper test dependency blocked.

### FIX-04 — IDE checks target a removed core service and obsolete hostnames

- **Finding/proof:** [`7301751e:backend/internal/service/workspaceide/service.go:18`](https://github.com/futrx-com/remote.futrx/blob/7301751e4128133489fe2cbca9db7aec110b9b53/backend/internal/service/workspaceide/service.go#L18); current `applications/code-server/application.json` declares project application web port 8842.
- **Validation:** Confirmed integration incompatibility, not evidence of an exploit in the unfetched original design.
- **Modification:** `workspaceide/permissions.go` vocabulary; `applications/permissions.go: RequireWebAccess`; application launch/direct proxy checks; `auth/access.go: VerifyWorkspace`; `auth_verify_handler.go: previewCapability`; current service/registry composition.
- **Invariant/reason:** Enforce service-owned checks at current Code Server admission and forward-auth preview access. Retain session registration and project membership. Do not restore the removed core IDE service.
- **Tests/results:** Current IDE launch/direct requests allowed or denied; nonmember/missing-session gateway coverage; real policy preview revocation; Terminal real WebSocket handshake/revocation plus shared application gateway coverage passed.
- **Status:** Fixed. Existing WebSocket connections are not forcibly terminated.

### FIX-05 — Provider-account scope and credential authorization require canonical integration

- **Finding/proof:** [`1b92d185:backend/internal/service/agent/auth/account_service.go:181`](https://github.com/futrx-com/remote.futrx/blob/1b92d1856698158308fd30e4fd2912af3b0f9ded/backend/internal/service/agent/auth/account_service.go#L181) and authorization at line 188.
- **Validation:** Confirmed intended security change requiring scope/model/UI reconciliation and grant provisioning.
- **Modification:** `rbac/models/definition.go, models.go`; `agent/auth/permissions.go, account_service.go`; `agent/accountaccess`; provider adapters; chat/prompt account checks; auth/quota HTTP and WebSocket filters; canonical account-target endpoint and scope forms.
- **Invariant/reason:** Authorize the resolved actual account under credential locking. No denied explicit selection fallback, no blanket member grant. Preserve role/direct deny behavior and administrator recovery.
- **Tests/results:** Real-policy direct/role grant and deny, persisted scope isolation, default switching, concurrent credentials, metadata hiding, HTTP/WebSocket filtering, revocation and targeted race tests passed.
- **Status:** Fixed. Deployment owners must provision exact account grants; rollback constraints are documented in docs/dev/permissions.md.

### FIX-06 — Old workspace middleware does not cover current Terminal application

- **Finding/proof:** [`eda6b1b4:backend/internal/transport/http/middleware/workspace.go:53`](https://github.com/futrx-com/remote.futrx/blob/eda6b1b40ccb760ab3f471468b201fe12fe382f3/backend/internal/transport/http/middleware/workspace.go#L53) and [`07462337:backend/internal/transport/http/handlers/applications_web_handler.go:79`](https://github.com/futrx-com/remote.futrx/blob/074623377c4057b25b697ad5059c6c2d578ba8e4/backend/internal/transport/http/handlers/applications_web_handler.go#L79)
- **Validation:** Confirmed architectural mismatch. Current application gateway had session/membership checks but no workspace capability check.
- **Modification:** `applications/permissions.go`; `applications_web_handler.go`; `auth_verify_handler.go`; `middleware/workspace.go` for current file/Git routes; project browser/secret/control gates; `transport.go: CanUploadToChat`; current UI controls.
- **Invariant/reason:** Check real application launch/proxy/WebSocket admission and preview ports before forwarding. Keep membership checks. Gate file upload admission as well as reads. Leave obsolete core terminal routes absent.
- **Tests/results:** Actual Terminal WebSocket echo and denied subsequent handshake; Code Server/Terminal direct and redirect admission; browser/IDE/Terminal preview checks with current policy; file/Git middleware denial before handler I/O; secret/control and upload-gate tests passed, including race checks.
- **Status:** Fixed. Active WebSockets and admitted tus uploads retain documented in-flight limitations.

### FIX-07 — Old audit/combined snapshots restore removed host tmux shell

- **Finding/proof:** Upstream removal commit `87f9842f3b6752eb9934d106bc235027e25539e4`; fetched #325 and #333 both still contain `tmux_handler.go`. [`d58aefc8:backend/internal/transport/http/server.go:42`](https://github.com/futrx-com/remote.futrx/blob/d58aefc832446d3689cb9b39873fbf1adaf9b025/backend/internal/transport/http/server.go#L42)
- **Validation:** Confirmed regression risk when replaying obsolete snapshots; current qa already satisfies removal.
- **Modification:** No old handler/socket restored. Current `http/server.go` and `transport.go` retained; added `transport/integration_routes_test.go`.
- **Invariant/reason:** No host session creation, command submission or tmux WebSocket route is registered. Current project Terminal application remains available through its gateway.
- **Tests/results:** Actual NewHTTPHandler composition rejects removed session/shell routes; source absence checks passed; current application gateway and real Terminal WebSocket regression passed.
- **Status:** Preserved and regression-tested; no restoration needed.

### FIX-08 — Audit stack lags newer #44 and assumes old services

- **Finding/proof:** [`36a0732b:backend/internal/service/audit/service.go:111`](https://github.com/futrx-com/remote.futrx/blob/36a0732bc65f8b781fd6cf41633f4436678edb7e/backend/internal/service/audit/service.go#L111); [`440965e9:backend/internal/service/audit/service.go:236`](https://github.com/futrx-com/remote.futrx/blob/440965e98f4b34278da09c3037238bc044162500/backend/internal/service/audit/service.go#L236)
- **Validation:** Confirmed integration incompatibility. #44 head includes newer composition and stale-response handling; #325 adds project filtering/export and account/run correlation.
- **Modification:** Latest `service/audit`, `stores/fileaudit`, audit HTTP/UI; current auth/user/project/chat/prompt/selfupdate instrumentation; account acquisition/mutation and application admission audit; transport actor/network context.
- **Invariant/reason:** Keep authenticated attribution, project/account/run/application correlation, filtering/export, retention and redaction. Operational events remain best effort; foundation permission-mutation audit remains mandatory before publication.
- **Tests/results:** Audit persistence/reopen/export/filter/retention, HTTP reads/export, prompt redaction and run outcome, account/application-owner correlation, best-effort failure and mandatory policy-audit failure tests passed. Persistence and authorization race checks passed.
- **Status:** Fixed.

### FIX-09 — Obsolete core scheduler conflicts with current Scheduled Tasks application

- **Finding/proof:** [`440965e9:backend/internal/service/schedule/service.go:800`](https://github.com/futrx-com/remote.futrx/blob/440965e98f4b34278da09c3037238bc044162500/backend/internal/service/schedule/service.go#L800) versus [`07462337:backend/internal/service/applications/agent_turns.go:194`](https://github.com/futrx-com/remote.futrx/blob/074623377c4057b25b697ad5059c6c2d578ba8e4/backend/internal/service/applications/agent_turns.go#L194)
- **Validation:** Confirmed integration incompatibility; current qa already migrated scheduling.
- **Modification:** Preserve `applications/agent_turns.go` and application-tool runtime; adapt `prompt/account_access.go`, prompt account tests and audit correlation to ApplicationInstanceID/ApplicationRequestID. Do not introduce service/schedule.
- **Invariant/reason:** Admission revalidates the application owner and membership. Prompt admission and credential acquisition evaluate that owner, not a trusted system identity. Application/run/account/project correlation survives.
- **Tests/results:** Current packaged-scheduler delivery/restart and application runtime tests passed in backend suite; catalog scheduled-task tests passed; prompt owner/account denial/revocation/audit tests and races passed.
- **Status:** Fixed without restoring core scheduling.

### FIX-10 — Older combined composition loses current routes

- **Finding/proof:** Fetched #333 `d58aefc8` still has `Schedules` and `TmuxWS`, and omits EmailSettings/AgentApplications; compare current qa `07462337` server and transport composition.
- **Validation:** Confirmed integration incompatibility on the fetched combined head.
- **Modification:** Preserve current `main.go`, `service/services.go`, `http/server.go`, `transport.go`; add permission/account/audit dependencies and registrations alongside EmailSettings and AgentApplications.
- **Invariant/reason:** Keep current initialization and application gateway architecture, injected auth/RBAC and supported upstream routes. Add no deleted-service dependency.
- **Tests/results:** Composition and HTTP package tests passed; actual NewHTTPHandler smoke test confirms EmailSettings and AgentApplications remain registered while removed routes stay absent. Both module vet runs passed.
- **Status:** Fixed.

### FIX-11 — Combined integration duplicates focused features and adds unrelated infrastructure

- **Finding/proof:** Fetched #333 head `d58aefc832446d3689cb9b39873fbf1adaf9b025`; first-parent history includes focused features plus `2191a12b` ZFS quota work and other storage/configuration branches.
- **Validation:** Confirmed integration-strategy incompatibility.
- **Modification:** No wholesale #333 merge and no unrelated storage/infrastructure edits. Selective feature patches were adapted onto current qa plus the current #223 foundation.
- **Invariant/reason:** Integrate each contribution once and preserve newer upstream architecture/security changes. Keep unrelated combined-stack work for its owners.
- **Tests/results:** Verified ancestry of the four supplied feature commits and audit integration, inspected selective #333 composition/history, checked final diff and removed-component absence.
- **Status:** Handled by selective integration; original dependent PR source branches remain unchanged.

## Integrated validation

- Backend `go test ./...`: rerun after repairing the fork attribution fixture.
  Every package passed except unchanged `internal/integration/containers/usersetup`:
  five tests fail because `timeout` is not installed on this macOS host. No test
  was skipped or weakened to hide this. Project service setup/secret propagation,
  failures/retries and lifecycle tests passed separately and in the integrated suite.
- Root/catalog `go test ./...`: passed, including current Scheduled Tasks packages.
- Backend and catalog `go vet ./...`: passed. The corrected filechat fixture also
  passed focused vet.
- Targeted `go test -race`: passed for RBAC, filepermissions, project lifecycle,
  agent account auth/accountaccess, prompt ownership, transport and application
  gateway tests. Additional full evaluator/fileaudit race tests and the corrected
  persisted-author fork race test passed.
- Frontend `npm test`: 584 passed, zero failed/skipped; `npm run build`: passed
  with the exact merged lockfile dependencies (Vite 8.3.5). Existing Vite native
  config and bundle-size warnings remain.
- `bash infra/tests/caddy-template-test.sh`: passed.
- Application UI JavaScript tests for Code Server and Terminal plus Terminal web
  reconnection: 29 passed.
- Linux-only Terminal test binary cross-compilation: passed with
  `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test -c`. Linux PTY execution was not
  possible on this macOS host; a real proxied WebSocket admission/echo/revocation
  test ran locally against an isolated test upstream.
- Real HTTP creation, management, scope isolation and wire-casing checks passed.
  Real WebSocket admission/revocation and actual composed-server registration
  regression checks passed.
- Modified Go formatting, conflict-marker scans and unmerged-file checks: clean.
  Working-tree `git diff --check` and `git diff --check a8eb4dbc HEAD` for the
  repair delta are clean. `git diff --check upstream/qa HEAD` reports inherited
  trailing whitespace in `.claude/skills/code-refactorer/SKILL.md` and its
  `references/workflow/commits.md`, already present in #223. These unrelated
  foundation documents were preserved, not silently rewritten.

Local logs: `/tmp/stack-final-backend-recheck.log`,
`/tmp/stack-final-catalog.log`, `/tmp/stack-final-vet-backend.log`,
`/tmp/stack-final-vet-catalog.log`, `/tmp/stack-final-race.log`,
`/tmp/stack-final-race-persistence.log`, `/tmp/stack-final-race-filechat.log`,
`/tmp/stack-final-frontend-test.log`, `/tmp/stack-final-frontend-build.log`,
`/tmp/stack-final-caddy.log`, `/tmp/stack-final-application-js.log`,
`/tmp/stack-final-terminal-cross.log`. These logs are local and not committed.

## Commits and destination

Implementation head before this documentation commit:
`2adb79708348cf1aea11c8a3a8c134822f0fe86f`.
The final documentation commit records this report and grant/rollback contracts;
use the integration branch head as the complete deliverable.

```text
bf032fa9 chore(merge): integrate current RBAC foundation
c6d9c9b0 fix(rbac): reconcile effective decisions and creation gates
0eb72219 fix(authz): guard current IDE and workspace application boundaries
24f0fa17 fix(accounts): integrate provider-account authorization
87cd55ea fix(audit): adapt current trail and account correlation
2adb7970 fix(authz): complete workspace admission and regression coverage
```

Each new integration commit has Samuel Emad as author and committer and exactly:

```text
Co-authored-by: Abdallah mohamed <abdoooomohamed88@gmail.com>
Co-authored-by: Samuel Emad <samuelemad49@gmail.com>
```

Publication destination: the new
`AbdallahMohamedDotnet/remote.futrx:review/rbac-dependency-reconciliation` branch,
using a normal push through the previously verified SSH credential. No PR is
opened, closed or merged. No original PR branch, upstream qa or main is updated.
Publication is **blocked**: automatic approval review rejected the push because
publishing repository history to this external fork/destination requires explicit
user authorization; the task's conditional push wording was not accepted as that
authorization. No push occurred. The complete integration remains in local
commits. After explicit approval, the exact normal push command is:

```bash
git -c core.sshCommand='ssh -o BatchMode=yes -o StrictHostKeyChecking=yes' push -u git@github.com:AbdallahMohamedDotnet/remote.futrx.git HEAD:refs/heads/review/rbac-dependency-reconciliation
```

The final chat report records the full local head SHA. Original PR branches and
upstream qa/main are unchanged.

## Remaining limits and owner actions

- Install GNU timeout in a suitable test environment and rerun the guest-wrapper
  integration tests. Execute Linux Terminal PTY tests on Linux.
- No live QA VM deployment, actual Caddy/LXD/container setup execution, provider
  credential login or live browser UI smoke test was performed in this task.
  Local proxy, policy and application runtime tests do not replace those checks.
- Existing WebSockets are not forcibly disconnected on revocation; subsequent
  requests/handshakes recheck policy. Admitted tus uploads retain upstream
  in-flight behavior. These are documented limitations, not claims of immediate
  session termination.
- Provision exact provider-account grants for intended users and scheduled-task
  owners before deployment. Follow `docs/dev/permissions.md` before policy
  rollback; old binaries can reject new keys/scopes, and restoring old snapshots
  can undo newer revocations.
- Montaserz owns #315/#317/#320/#324/#325/#333. Those source branches were not
  updated. Their owner must reconcile them with this integration or choose this
  branch as the replacement integration vehicle, avoiding duplicate feature merges.
- #223 remains on its existing verified source head. #44 source is unchanged.
  No unrelated #333 storage, ZFS, infrastructure or configuration work was imported.
