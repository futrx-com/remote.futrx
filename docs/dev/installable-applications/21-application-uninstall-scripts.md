# Application uninstall scripts

## Removing a project installation

`uninstall` is optional and explicit; merely including `infra/uninstall.sh`
does not enable it. The catalog validates the path and loads its bytes. During
project teardown Remote removes any host proxy, requests service shutdown,
runs the declared script, removes generated service files and skills, then
removes backend state and the instance record through the service layer.

The script runs as root through `bash -s` with the same resolved application
environment as installation, an eight-minute execution timeout, and captured
output. It runs in the existing project container; it must never delete other
applications' files or shared project data. Retry of a failed install uses the
same teardown and can therefore run this cleanup too.

If the container exists but is stopped, cleanup returns an error asking the
operator to start the project container. If it is missing, the script is
skipped; that does not remove files that may still exist in persistent
workspace storage. A script failure prevents later teardown steps and deletion
of the instance record, but earlier stop/proxy operations are not rolled back.
The record's previous status is not automatically changed by uninstall failure.
At global scope, Remote deletes the dedicated container and does not run this
script. Without an uninstall script, package/data cleanup remains the app
author/operator's responsibility. See [04 — Install scripts](04-install-scripts.md).

## Source and verification

- [backend/internal/integration/containers/applications/installer.go](../../../backend/internal/integration/containers/applications/installer.go)
- [backend/internal/integration/containers/applications/installer_test.go](../../../backend/internal/integration/containers/applications/installer_test.go)
- [backend/internal/integration/containers/applications/registry.go](../../../backend/internal/integration/containers/applications/registry.go)
- [backend/internal/integration/containers/applications/registry_infra.go](../../../backend/internal/integration/containers/applications/registry_infra.go)
- [backend/internal/integration/containers/applications/registry_manifest.go](../../../backend/internal/integration/containers/applications/registry_manifest.go)
- [backend/internal/integration/containers/applications/registry_test.go](../../../backend/internal/integration/containers/applications/registry_test.go)
- [backend/internal/integration/containers/applications/registry_validation.go](../../../backend/internal/integration/containers/applications/registry_validation.go)
- [backend/internal/service/applications/model.go](../../../backend/internal/service/applications/model.go)
- [frontend/src/models/application.ts](../../../frontend/src/models/application.ts)
- [frontend/src/ui/applications/applicationPresentation.test.ts](../../../frontend/src/ui/applications/applicationPresentation.test.ts)
- [frontend/src/ui/applications/applicationPresentation.ts](../../../frontend/src/ui/applications/applicationPresentation.ts)

The included unit tests verify decisions and generated requests/commands.
No live container installation, authenticated browser flow, or QA deployment
was performed for this split.

## What to verify

Tests check invalid or missing cleanup scripts, catalog availability, cleanup
order, script errors and missing containers. The frontend confirmation test
checks the cleanup explanation. Verify real files/packages, a failing script,
a stopped container and Retry after partial installation.

### Responsibility boundaries

- [installer_uninstall.go](../../../backend/internal/integration/containers/applications/installer_uninstall.go) — Owns uninstall ordering and execution of the application cleanup script, including missing/stopped-container handling.
