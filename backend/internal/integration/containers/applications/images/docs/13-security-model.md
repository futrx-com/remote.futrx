# 13 — Security model

## The trust boundary is the build, not the request

Extension code runs **on the main origin with the same privileges as the SPA**.
There is no sandbox, and none is implied. A `ui/` directory can:

- read and write the DOM of the whole application;
- call any Remote API endpoint as the signed-in user;
- read `localStorage`, and anything else same-origin JavaScript can reach.

What it cannot do is get there without a build. Assets are compiled into the
server binary by `//go:embed`, next to the SPA itself. There is no runtime
plugin directory, no upload endpoint, and no way to add an image to a running
server. Someone who can add a `ui/` directory can already ship arbitrary
frontend code by editing `frontend/src`.

**Therefore: treat a new or edited `ui/` in a pull request exactly as you treat
any other frontend change.** That is the control. Reviewing an image's
`install.sh` carefully while skimming its `ui/` gets the risk backwards — the
script runs in a disposable container, the extension runs in the user's
session.

## What is enforced, and where

| Property | Enforced by | Notes |
|---|---|---|
| Only catalog images exist | `//go:embed` | No runtime installation |
| A malformed image cannot ship | `registry.go:validate`, `registry_ui.go:loadImageUI` | Fails the build and the tests |
| Assets stay inside one image's `ui/` | `registry.go:cleanUIPath` | The only path out of the package |
| Only signed-in users fetch assets | `applications_handler.go` | Same gate as the catalog |
| Responses are not sniffable | `Content-Type` from extension + `nosniff` | Types are pinned, never guessed |
| Uninstalled extensions do not load | `Service.UIExtensions` | Installation is the gate |
| Project extensions stay in their project | `ExtensionRegistry.isInScope` | Frontend, per contribution, per render |
| A project you cannot see never reaches you | `ListVisible(email, isAdmin)` | Backend; the frontend never sees those entries |
| Global app management is admin-only | `requireAdmin` | Server-wide infrastructure |
| A project member cannot touch another project's app | `ensureProject` | Ownership re-checked per request |
| Secrets are not echoed to the UI | `View` / `envPublic` | Secret env values are redacted outside the credentials route |

## Path traversal

`Registry.UIAsset` is the only way `ui/` bytes leave the integration package,
and it is where traversal stops:

```go
func (r *Registry) UIAsset(imageID, assetPath string) ([]byte, bool)
```

It rejects an unknown image, an image with no `ui/`, an empty or absolute path,
anything containing a backslash, and any path that resolves outside the image's
own `ui/` after cleaning. Both `..` and percent-encoded `%2e%2e` are covered,
because the check runs on the decoded, cleaned path.

Verified requests, all `404`:

```
/api/applications/catalog/mysql/ui/../install.sh
/api/applications/catalog/mysql/ui/../../redis/install.sh
/api/applications/catalog/mysql/ui/%2e%2e/install.sh
/api/applications/catalog/mysql/ui/..%2finstall.sh
/api/applications/catalog/redis/ui/scripts/main.js      (redis ships no ui/)
```

Pinned by `registry_test.go:TestCleanUIPath` and `TestRegistryUIAsset`, and
re-checked at runtime by the `ui-playground` self-test.

## Serving HTML same-origin

Views are served as `text/html` on the application's own origin, so a view is
in principle a same-origin page. This is safe **only because of the build
boundary**: those bytes were compiled into the binary by whoever built the
server. It is not safe reasoning if images ever become runtime-installable —
see below.

## Two flavours of "safe"

The extension system is carefully **robust**: a throwing entry module, render,
predicate, or click handler is caught, and costs one extension its own UI.

That is not a security boundary. A malicious extension does not need to throw —
it can simply do the harmful thing correctly. Robustness protects against bugs;
the build boundary protects against malice.

## If images ever become runtime-installable

Everything above changes. The build boundary is what makes the current design
sound; remove it and you need, at minimum:

- **Isolation.** An iframe on a separate origin with a `postMessage` bridge,
  rather than direct DOM and `fetch` access. Slots would post render intents
  instead of receiving elements.
- **A capability model.** Explicit, reviewable permissions per extension rather
  than the ambient authority of the signed-in session.
- **CSP.** A policy that stops an extension reaching arbitrary third-party
  origins.
- **Signing and provenance.** Some answer to "who wrote this and did it change".

None of that exists today, deliberately, because none of it is needed for
build-time code. Do not add runtime installation without it.

## Reviewing an extension

A checklist for reviewing a `ui/` directory:

- **`innerHTML` with interpolated data.** Views are static markup, but anything
  built from API responses or user input needs escaping. `ui-playground` has an
  `escapeHtml` helper for exactly this.
- **Where does it send data?** `fetch` to anything not same-origin is
  exfiltration surface. There is no CSP stopping it.
- **What does it read?** Credentials endpoints return real passwords. An
  extension reading them is legitimate (MySQL's does) — but it should display
  them, not transmit them.
- **Does it touch the app's DOM outside its host elements?** Unsupported, will
  break, and may be doing something it should not.
- **Does the install scope match the intent?** A plugin meant for one project
  should not be documented as a global install.

## Related

- [12 — HTTP API](12-http-api.md) — the authorization of each endpoint.
- [08 — Scoping and visibility](08-scoping-and-visibility.md) — who sees what.
- The platform-wide [threat model](../../../../../../../docs/threat-model.md) — the
  boundaries this sits inside.
