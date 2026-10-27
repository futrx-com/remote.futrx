# JSON installation settings

## Editing an installation settings object

`defaultFile` is read once when the catalog loads from a UTF-8 file under the
application's `infra/` directory. It becomes `env[].default` in the catalog
response; `defaultFile` itself is cleared. A file may contain at most 128 KiB,
must exist, and cannot be combined with a nonempty inline `default`.

`format: "json"` makes the install dialog wider and renders a multiline
editor initialized from the default. The browser checks JSON syntax, object
shape, and the 128-KiB UTF-8 size limit before submitting. The server applies
defaults/generators to blank inputs and independently validates resolved JSON
values. Arrays, `null`, primitives, comments, and malformed JSON are rejected;
`{}` is a valid object. Invalid settings produce HTTP 400. A malformed default
also prevents catalog admission. These are strings containing JSON, not nested
objects in the install request's `env` map.

The resolution order is trimmed supplied value, then default, then password
generator, then required-field rejection. An optional value can stay absent.
`secret: true` redacts the saved value from ordinary instance views; it does
not mask the JSON editor, remove the package default from the catalog response,
or restrict the existing credentials endpoint beyond its authorization.
Do not put actual credentials into a package's default settings file.

The platform does not decide where settings are written inside the container
or how they are edited after installation. That work belongs to the package's
install script and, if needed, its application backend.

## Source and verification

- [backend/internal/integration/containers/applications/registry.go](../../../backend/internal/integration/containers/applications/registry.go)
- [backend/internal/integration/containers/applications/registry_infra_test.go](../../../backend/internal/integration/containers/applications/registry_infra_test.go)
- [backend/internal/integration/containers/applications/registry_validation.go](../../../backend/internal/integration/containers/applications/registry_validation.go)
- [backend/internal/service/applications/environment_test.go](../../../backend/internal/service/applications/environment_test.go)
- [backend/internal/service/applications/install.go](../../../backend/internal/service/applications/install.go)
- [backend/internal/service/applications/model.go](../../../backend/internal/service/applications/model.go)
- [backend/internal/service/applications/service.go](../../../backend/internal/service/applications/service.go)
- [backend/internal/transport/http/handlers/applications_handler.go](../../../backend/internal/transport/http/handlers/applications_handler.go)
- [frontend/src/models/application.ts](../../../frontend/src/models/application.ts)
- [frontend/src/services/applications/prepareInstallRequest.test.ts](../../../frontend/src/services/applications/prepareInstallRequest.test.ts)
- [frontend/src/services/applications/prepareInstallRequest.ts](../../../frontend/src/services/applications/prepareInstallRequest.ts)
- [frontend/src/ui/applications/ApplicationCatalog.tsx](../../../frontend/src/ui/applications/ApplicationCatalog.tsx)

The included unit tests verify decisions and generated requests/commands.
No live container installation, authenticated browser flow, or QA deployment
was performed for this split.

## What to verify

Tests check default-file loading/path rejection, malformed JSON, arrays, null,
primitives, default selection and browser request preparation. Oversized-value
rejection is implemented but not exhaustively tested by these existing tests.
Verify valid and invalid direct API requests and the multiline editor.

### Responsibility boundaries

- [registry_environment.go](../../../backend/internal/integration/containers/applications/registry_environment.go) — Loads and validates default files before catalog validation.
- [environment.go](../../../backend/internal/service/applications/environment.go) — Validates structured environment values without performing installation.
- [useApplicationInstallForm.ts](../../../frontend/src/state/hooks/applications/useApplicationInstallForm.ts) — Owns form state and submission; the dialog renders fields and the request service validates the payload.
