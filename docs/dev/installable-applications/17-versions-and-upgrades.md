# 17 — Versions and upgrades

Every application declares a `version` in its `application.json`. It is required, and it is
not decoration: it is the signal that decides whether an already-installed copy
of an application's container side is converged again.

```json
{ "id": "s3disk", "name": "s3disk", "version": "0.2.0", … }
```

## The rule

An installed instance records the version of the application it was installed from
(`applicationVersion`). When the catalog's version for that application differs from what
an instance recorded, that instance's container side is stale. Remote rebuilds
container programs, re-runs any custom install script, and rematerializes the
manifest service against that same instance.

Two things follow from "differs" rather than "is newer":

- **Versions are free text.** `"8.0"`, `"16"`, `"1.2.3-rc1"`, `"2024-11-02"`.
  No ordering is parsed and none is invented, because there is no format all
  applications agree on. A version that changes in either direction re-installs; a
  deliberate rollback is therefore a real rollback.
- **Not bumping the version means "nothing in a container changed".** That is
  the author's control. Re-upload the same version as often as you like while
  iterating on UI or host-backend source; those edits alone do not reinstall
  the container. Changed `backend/container/` source or a replaced container
  still requires convergence independently of the manifest version.

## What re-runs, and what does not

| Part of an application | When it refreshes |
|---|---|
| `infra/install.sh` | Initial install, failed-install Retry, version/container-build upgrade, running-app restoration after container replacement, or Start when the declared service-unit check fails |
| Manifest `service` | Materialized on install, upgrade and restoration; normal Start uses its service or socket, and reinstalls if the declared service unit is missing |
| `backend/container/` programs | On install/restoration when their build marker is absent, or when their source digest/application version differs |
| `ui/` assets | Every upload — they are served from the catalog, not a container |
| `backend/` Go source | Every upload — the backend process is stopped and rebuilt on its next call, event, or eligible background recovery |
| Catalog metadata (name, description, env fields, scopes) | Every upload |

A UI-only or host-backend-only application reaches no container at all, so a
version bump on one changes the catalog entry and nothing else.

The agent runtime flags are manifest declarations refreshed from the current
catalog. `background` recovery applies only to running installations, while
`agentTurns` and `agentTools` independently opt into execution and tools.
Stop/start and upgrades retain application state and execution receipts;
uninstall removes both. See [Application agent runtime](25-application-agent-runtime.md).

## When the upgrade happens

**On upload**, for every copy that is *running* or in *error*. Uploading a
package with a new version re-provisions them immediately, in every scope —
the global install and each project's install.

**On start**, for a copy that is *stopped*. Re-running an install script also
brings the app up, and an upload has no business resurrecting an app someone
deliberately stopped. A stopped copy therefore keeps its old version, is
badged in the UI with the version it will move to, and upgrades the moment its
owner starts it. This is also how a copy that was down during a Remote release
catches up with a built-in application that changed version.

## Container replacement recovery

`RestoreProject(projectID)` re-runs installation for persisted `running`
applications with container capabilities after project container recreation,
even when their version is unchanged. The project start path calls it after
recreating a missing container; `cmd/upgrade-workspaces` calls it after a
successful workspace upgrade and loads uploaded packages as well as the
embedded catalog. Stopped/error/installing instances and apps without container
capabilities are skipped.

Recovery rechecks each instance under a nonblocking lifecycle lock. A busy
instance is skipped without queuing a retry. It reconciles saved env values
against the current manifest, stops the old host backend, installs into the
replacement container, and records current application/build versions. The
backend comes back on its next authorized call. Individual failures are
collected so other applications can still be restored.

Project start logs recovery failures without failing the whole project start.
The upgrade command counts recovery failure as a failed workspace upgrade,
without rolling the new container back. Recovery does not restore arbitrary
application data; scripts must use durable storage and tolerate repeated runs.

A stopped application remains stopped. When explicitly started, its declared
service unit is checked; a failed existence check invokes the full installer.
This is not complete package detection and does not repair a script-only app
with no service declaration merely because its files disappeared. See the
[state and failure table](24-application-container-recovery.md#recovering-after-container-replacement).

## What an upgrade preserves

A re-install replaces software, not identity. The instance keeps:

- its container — the dedicated one for a global install, the project's for a
  project install;
- its host port and bind address, so nothing that already connects to it has to
  be repointed;
- values for still-declared environment keys, including generated passwords;
  new keys receive defaults/generators and removed keys are discarded.

Clients keep the same connection address, but service/backend restarts and
application-owned migrations can still interrupt them.

## Install scripts must be idempotent

This is not new — [04 — Install scripts](04-install-scripts.md) already
requires it — but upgrades are what make it load-bearing. A script that
re-provisions a database it already provisioned must converge rather than fail,
duplicate, or wipe data.

The same requirement is why an instance with **no** recorded version
(installed before versions were tracked) is re-installed rather than assumed
current: running an idempotent script unnecessarily is cheap, and assuming a
container already holds software it may not is a guess about someone's data.

## When an upgrade fails

The upload still succeeds — the package is stored and the catalog entry is
live. The instance that failed:

- is left in the **error** state with the script's output as its error;
- **keeps its old recorded version**, so it is retried by the next upload or by
  a manual start, rather than skipped as already current.

Per-instance results come back on the upload response as `upgraded[]`, and the
UI reports them under the upload:

```json
{
  "id": "s3disk", "version": "0.2.0",
  "upgraded": [
    { "instanceId": "a1b2", "name": "s3disk", "scope": "project",
      "projectId": "proj-7", "fromVersion": "0.1.0", "toVersion": "0.2.0" },
    { "instanceId": "c3d4", "name": "s3disk", "scope": "project",
      "projectId": "proj-9", "fromVersion": "0.1.0", "toVersion": "0.2.0",
      "error": "install script exited 1: …" }
  ]
}
```

## Built-in applications

The same mechanism, the same recorded version. What differs is the trigger:
there is no upload event for an application compiled into the binary, so a running
instance of a built-in application is not re-provisioned by a Remote release on its
own — `infra/update.sh` owns that, and recycles workspaces deliberately rather
than re-running install scripts across every container at boot. A stopped
instance still upgrades on its next start.

## Related

- [16 — Uploaded packages](16-uploaded-packages.md) — how a package gets into
  the catalog in the first place.
- [04 — Install scripts](04-install-scripts.md) — the idempotency contract this
  depends on.
- [02 — application.json reference](02-application-json.md) — the `version` field.
