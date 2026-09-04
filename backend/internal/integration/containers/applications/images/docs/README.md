# Installable images and UI extensions

This is the complete documentation for the **applications catalog**: the
one-click installable apps in Remote, and the browser extensions they can ship
to change the Remote interface itself.

Two things live here, and the difference matters:

- **An application** installs software — MySQL, PostgreSQL, Redis — into a
  container and exposes it on a port.
- **A UI extension** installs nothing anywhere. It is a `ui/` directory that
  runs in the browser and contributes to defined places in the Remote
  interface: a button in the chat header, a panel, a popup.

One image can be either, or both. A MySQL image can ship a "Connect" button
alongside the database it provisions; a plugin that only adds a button ships no
container side at all.

## Start here

| If you want to… | Read |
|---|---|
| Understand how the whole thing fits together | [01 — Overview](01-overview.md) |
| Add a database or service to the catalog | [03 — Image types](03-image-types.md), [04 — Install scripts](04-install-scripts.md) |
| Add a button, panel, or popup to the Remote UI | [07 — Tutorial](07-tutorial-build-a-plugin.md) |
| Look up a field in `image.json` | [02 — image.json reference](02-image-json.md) |
| Look up an extension API method | [06 — Extension API reference](06-extension-api.md) |
| Know where you are allowed to render | [05 — Slots](05-slots.md) |
| Know who sees your extension | [08 — Scoping and visibility](08-scoping-and-visibility.md) |
| Make your UI match the app's look | [09 — Styling and icons](09-styling-and-icons.md) |
| Test what you built | [10 — Fixtures](10-fixtures.md), [11 — Testing](11-testing.md) |
| Call the endpoints directly | [12 — HTTP API](12-http-api.md) |
| Understand the trust model | [13 — Security model](13-security-model.md) |
| Fix something that is not working | [14 — Troubleshooting](14-troubleshooting.md) |

## All documents

1. [Overview](01-overview.md) — the moving parts, and how a request flows through them.
2. [image.json reference](02-image-json.md) — every field, with types and defaults.
3. [Image types](03-image-types.md) — `service` vs `ui`, and which one creates a container.
4. [Install scripts](04-install-scripts.md) — the contract, the environment, idempotency.
5. [Slots](05-slots.md) — every place an extension may render, and the context each provides.
6. [Extension API reference](06-extension-api.md) — the complete `remote` object.
7. [Tutorial: build a plugin](07-tutorial-build-a-plugin.md) — end to end, from empty directory to working button.
8. [Scoping and visibility](08-scoping-and-visibility.md) — install scope is render scope.
9. [Styling and icons](09-styling-and-icons.md) — design tokens, theming, the icon field.
10. [Fixtures](10-fixtures.md) — `ui-playground` and `ui-sandbox`, and how to test with them.
11. [Testing](11-testing.md) — unit tests, the self-test, and browser verification.
12. [HTTP API](12-http-api.md) — every endpoint, its shape, and its authorization.
13. [Security model](13-security-model.md) — what is enforced, where, and what is not.
14. [Troubleshooting](14-troubleshooting.md) — symptoms, causes, fixes.

## Conventions in these documents

Paths are given relative to the repository root unless stated otherwise. The
catalog lives at:

```
backend/internal/integration/containers/applications/images/
```

and is symlinked from the repository root as `installable-images/`, so
`installable-images/mysql/image.json` and the long path above are the same
file.

Code references name the file and, where useful, the symbol —
`registry_ui.go:loadImageUI`, `extensionContributionState.ts:visibleExtensionContributions`.
