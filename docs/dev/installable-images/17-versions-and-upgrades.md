# 17 — Versions and upgrades

Every image declares a `version` in its `image.json`. It is required, and it is
not decoration: it is the signal that decides whether an already-installed copy
of an application has its install script run again.

```json
{ "id": "s3disk", "name": "s3disk", "version": "0.2.0", … }
```

## The rule

An installed instance records the version of the image it was installed from
(`imageVersion`). When the catalog's version for that image differs from what
an instance recorded, that instance's container side is stale — the script that
provisioned it belonged to a different release — and the install script is run
again against it.

Two things follow from "differs" rather than "is newer":

- **Versions are free text.** `"8.0"`, `"16"`, `"1.2.3-rc1"`, `"2024-11-02"`.
  No ordering is parsed and none is invented, because there is no format all
  images agree on. A version that changes in either direction re-installs; a
  deliberate rollback is therefore a real rollback.
- **Not bumping the version means "nothing in a container changed".** That is
  the author's control. Re-upload the same version as often as you like while
  iterating on a `ui/` or a `plugin/`; no container is touched.

## What re-runs, and what does not

| Part of an image | When it refreshes |
|---|---|
| `install.sh` (a `service` or `tool`) | Only when the version differs |
| `ui/` assets | Every upload — they are served from the catalog, not a container |
| `plugin/` Go source | Every upload — the plugin process is stopped and rebuilt on its next call |
| Catalog metadata (name, description, env fields, scopes) | Every upload |

A `ui` or `backend` image reaches no container at all, so a version bump on one
changes the catalog entry and nothing else. There is no install script for it
to re-run.

## When the upgrade happens

**On upload**, for every copy that is *running* or in *error*. Uploading a
package with a new version re-provisions them immediately, in every scope —
the global install and each project's install.

**On start**, for a copy that is *stopped*. Re-running an install script also
brings the app up, and an upload has no business resurrecting an app someone
deliberately stopped. A stopped copy therefore keeps its old version, is
badged in the UI with the version it will move to, and upgrades the moment its
owner starts it. This is also how a copy that was down during a Remote release
catches up with a built-in image that changed version.

## What an upgrade preserves

A re-install replaces software, not identity. The instance keeps:

- its container — the dedicated one for a global install, the project's for a
  project install;
- its host port and bind address, so nothing that already connects to it has to
  be repointed;
- its resolved environment, including generated passwords.

Which means an upgrade is invisible to clients apart from the restart the
install script itself performs.

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

## Built-in images

The same mechanism, the same recorded version. What differs is the trigger:
there is no upload event for an image compiled into the binary, so a running
instance of a built-in image is not re-provisioned by a Remote release on its
own — `infra/update.sh` owns that, and recycles workspaces deliberately rather
than re-running install scripts across every container at boot. A stopped
instance still upgrades on its next start.

## Related

- [16 — Uploaded packages](16-uploaded-packages.md) — how a package gets into
  the catalog in the first place.
- [04 — Install scripts](04-install-scripts.md) — the idempotency contract this
  depends on.
- [02 — image.json reference](02-image-json.md) — the `version` field.
