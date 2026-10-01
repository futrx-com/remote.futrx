# Requirements contribution and validation

This integration combines the focused requirements PRs, preserving upstream
RBAC #223 and audit-log #44 commits. It resolves the shared configuration,
composition and documentation conflicts. Use this combined head for QA, or
merge/rebase the focused contributions in dependency order; do not cherry-pick
the same changes twice.

| Requested behavior | Implementation / focused PRs |
| --- | --- |
| Restrict project/chat creation and administer users/roles | Existing RBAC policies, scopes, assignments and effective decisions; #315 |
| Restrict IDE, terminal, files, browser, secrets and project controls | Service/HTTP/WebSocket/forward-auth checks plus UI controls; #317, #324 (including application web origins on the combined head) |
| Assign multiple Claude/Codex accounts to users/roles; hide unauthorized accounts | Existing account vaults and per-chat run homes, account-scoped RBAC, filtered catalogs/quota/status and credential-time checks; #320 |
| Persist user identity with prompts/runs and display it under prompts | Durable chat events, run records and forked history; #311 |
| Audit creation, prompts/runs, IDE/terminal, Git, secrets, account usage and lifecycle; filter by user/project/action/date | Existing append-only audit store extended with project/run/account correlation, admin UI and filtered export; #325 |
| Configure software in new/recreated containers | Validated operator manifest, image digest/rebuild and optional browser/IDE prebaking; #328 |
| Trim installation/build caches and stale agy binaries | Cleanup before image publication, including after optional software installation; #313, #328 |
| Configure global/provider-specific instructions and preserve project supplements | Existing provider targets and composer, file configuration and admin editor; #310, #331 |
| Select COW storage/source and enable compression | Installer auto-detection or explicit ZFS/Btrfs/dir selection, preserved existing pools; #312 |
| Move existing containers to a new pool | Migration guide plus stopped-instance backup/preflight/move helper; #312, #330 |
| Apply default root quotas and show project disk usage/host warnings | Driver-aware 20GiB default, effective root-pool detection, bounded background persistent usage scans, byte/inode warnings at configurable 80%; #330 |
| Avoid growing root caches where practical | Per-project persistent npm/Go cache defaults with explicit overrides; general ~/.cache left intact to preserve installed assets; #328 |

## Automated validation

On the combined branch:

- Full backend `go test -timeout 3m ./...` passes, including application integration and composition tests.
- Root/catalog `go test ./...` passes.
- Frontend production build and all 547 frontend tests pass.
- Race tests pass for RBAC, account access, chat/project/prompt services, account vault/execution, audit/chat stores, HTTP transport, resource quotas, storage metrics and instruction settings.
- Backend `go vet ./...` passes.
- Installer storage selection, LXD host environment and QA script tests pass.
- Four simulated migration tests pass: read-only preflight, backup before move, rejection of running instances and no move after backup failure.

Upstream qa `6829779d` is merged, retaining its disk-capacity admission checks
and application recovery/origin isolation. Application web origins additionally
check browser RBAC with a verified-session actor.

Cross-feature checks caught and fixed a fork-attribution fixture that lacked the
new RBAC actor, quota validation occurring after other resource mutations, and
duplicate cache environment keys. Permission defaults and account isolation are
covered by service/transport tests; these do not replace live provider testing.

## Activation and compatibility

- Grant member access to the intended Claude/Codex accounts after upgrading;
  account use is default-deny. Existing running providers are not forcibly
  terminated by revocation. See [permissions](../dev/permissions.md).
- Existing project members retain capability baselines unless explicitly denied.
  Platform host capabilities are admin by default.
- Save instructions in Settings → Agents, then restart the backend. Subsequent
  provisioning publishes the new managed files. Existing project instruction
  files remain untouched. See [instructions](10-agent-instructions.md).
- Rebuild the base image with the software manifest and recreate projects through
  Remote's normal lifecycle to apply it. See [sandbox software](13-sandbox-software.md).
- Existing pools are never silently converted. Migration is explicit and offline.
  See [storage setup](11-storage.md) and [disk safety](14-storage-safety.md).
- Root quotas do not cover host bind-mounted workspace/provider-home data. That
  filesystem needs its own capacity policy. The [persistent quota backend](16-persistent-project-quotas.md) now provides ZFS dataset enforcement for workspace/provider-home data; legacy directory storage explicitly reports no enforcement.
- Application permissions are not isolation from a project root shell or an
  agent already authorized to execute arbitrary commands in that same project.

## Executed isolated-VM acceptance — 2026-10-01

On a project-local Ubuntu 24.04 VM (kernel 6.8.0-142, ZFS 2.2.2, LXD
5.21.8), with file-backed disposable pools and no production credentials:

- Persistent ZFS quota: a real write was rejected at 32MiB; provider-home marker
  data and the existing quota survived manager recreation; a different project
  identity could not adopt the dataset.
- Root quota: Remote’s resource manager resolved the migrated ZFS pool, applied
  a 64MiB limit, and a real write exceeding it was rejected.
- Migration: the helper’s read-only preflight, backups and dir-to-ZFS move passed;
  root/workspace/provider-home markers and mount definitions survived. A stopped
  rollback to dir preserved the same data after removing the unsupported quota.
- Real Chromium isolation: launch redirects and application WebSockets worked;
  platform reads, writes, logout, WebSockets and navigation remained blocked
  from the application origin. Cookies were not forwarded upstream.

The storage tests are opt-in Go tests documented in the
[persistent quota guide](16-persistent-project-quotas.md). The small container
image tests filesystem behavior; it does not substitute for the full Remote
base image or real Claude/Codex authentication.

## Live release acceptance still required

No production deployment or storage mutation has been performed. Before release,
on disposable QA hosts:

1. Test both a fresh COW install and an upgrade preserving an existing dir pool.
2. Rebuild a customized image, recreate a project and verify declared software,
   caches, provider homes, instructions and application data survive as intended.
3. Exercise two users with different roles/accounts concurrently, direct URLs,
   WebSocket opens, account switching and permission revocation.
4. Inspect persisted prompt authors and correlated audit entries after restart,
   including combined filters and export.
5. Exhaust a disposable root quota, trigger capacity warnings, and confirm
   unsupported/unknown metrics are not reported as enforced/zero.
6. Move a populated stopped project from dir to ZFS/Btrfs, verify bind mounts,
   agent execution and applications, then demonstrate rollback. Retain backups
   and source pools until these checks finish.

The isolated storage/browser checks above are complete. Full installer/update,
customized Remote image and concurrent real-provider acceptance remain.
