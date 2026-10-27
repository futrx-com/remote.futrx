# Application container recovery

## Recovering after container replacement

The instance JSON is stored on the host and can still say `running` after a
project container's packages and unit files disappear. `RestoreProject` fixes
that mismatch at two call sites:

1. The project start path invokes its restorer after recreating a missing
   container and synchronizing secrets. Normal starts of an existing container
   do not invoke that recovery callback.
2. `cmd/upgrade-workspaces` invokes it after successfully upgrading a workspace.
   That command now loads both the embedded catalog and administrator-uploaded
   packages from `<dataDir>/app-packages`.

| Persisted state or condition | Recovery behavior |
|---|---|
| `running` and has container capability | Resolve current declared env values, stop the host backend, run the installer, record current version/build identity, keep `running` on success |
| `stopped`, `error`, or `installing` | Skip automatic restoration |
| UI-only or host-backend-only app | Skip container installation |
| Another lifecycle operation owns the instance lock | Skip this pass without waiting; no retry is queued by this method |
| Environment reconciliation, backend stop, or install fails | Attempt to record `error`, collect the failure, and continue with other instances |
| Catalog/instance lookup fails | Return that failure with the other failures; recovery cannot reconstruct an absent package |

Successful recovery stops the old host backend; its next authorized call can
start it again. Existing declared values are preserved, newly declared inputs
receive defaults/generators, and keys removed from the manifest are dropped.

Project start logs recovery failures and can still mark the project running.
The upgrade command counts a restoration failure as a failed workspace upgrade;
it does not roll the newly created container back. This is not a continuous
reconciliation loop or a backup/restore of application data.

A stopped app can be started later. The installer tests for the declared
`/etc/systemd/system/<service>.service`; if that command fails, it invokes the
full install path even when the app version matches. This detection relies on
a declared service. It is not an inventory check for every installed package,
setting or portless script-only application.

Packages must keep durable data in persistent storage and make install scripts
idempotent. See [17 — Versions and upgrades](17-versions-and-upgrades.md).

## Source and verification

- [backend/cmd/upgrade-workspaces/main.go](../../../backend/cmd/upgrade-workspaces/main.go)
- [backend/internal/integration/containers/applications/installer.go](../../../backend/internal/integration/containers/applications/installer.go)
- [backend/internal/integration/containers/applications/installer_test.go](../../../backend/internal/integration/containers/applications/installer_test.go)
- [backend/internal/service/applications/instance_locks.go](../../../backend/internal/service/applications/instance_locks.go)
- [backend/internal/service/applications/upgrade.go](../../../backend/internal/service/applications/upgrade.go)
- [backend/internal/service/applications/upgrade_test.go](../../../backend/internal/service/applications/upgrade_test.go)
- [backend/internal/service/project/service.go](../../../backend/internal/service/project/service.go)
- [backend/internal/service/project/service_start_test.go](../../../backend/internal/service/project/service_start_test.go)
- [backend/internal/service/services.go](../../../backend/internal/service/services.go)

The included unit tests verify decisions and generated requests/commands.
No live container installation, authenticated browser flow, or QA deployment
was performed for this split.

## What to verify

Tests check running versus stopped restoration, busy instance-lock skipping,
missing-container project-start callbacks and missing-unit reinstall on Start.
Not every failure combination is covered. Replace a disposable container with
running/stopped apps, then check restoration and explicit stopped-app Start.

### Responsibility boundaries

- [restore.go](../../../backend/internal/service/applications/restore.go) — Owns project restoration and the per-instance lock; upgrade.go continues to own package-version upgrades.
