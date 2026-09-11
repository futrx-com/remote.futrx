# Installable images and UI extensions

This is the complete documentation for the **applications catalog**: the
one-click installable apps in Remote, and the browser extensions they can ship
to change the Remote interface itself.

Four things live here, and the differences matter:

- **An application** installs software — MySQL, PostgreSQL, Redis — into a
  container and exposes it on a port.
- **A workspace tool** installs software into the project's container and
  exposes nothing — a CLI, a mount, an agent. It is useful because it is *in*
  the workspace, not because anything can connect to it.
- **A UI extension** installs nothing anywhere. It is a `ui/` directory that
  runs in the browser and contributes to defined places in the Remote
  interface: a button in the chat header, a panel, a popup.
- **A backend plugin** is a `plugin/` directory of Go source. The server
  compiles it and runs it as a process, and the image's `ui/` calls it. It is
  how an image adds a server-side feature rather than only a button.

One image can be any of these, or several at once. A MySQL image can ship a "Connect"
button alongside the database it provisions and a plugin that runs the queries
behind it; an image that only adds a button ships no container side at all.

## Start here

| If you want to… | Read |
|---|---|
| Understand how the whole thing fits together | [01 — Overview](01-overview.md) |
| Add a database or service to the catalog | [03 — Image types](03-image-types.md), [04 — Install scripts](04-install-scripts.md) |
| Add a tool to the project workspace | [03 — Image types](03-image-types.md#tool), [04 — Install scripts](04-install-scripts.md) |
| Add a button, panel, or popup to the Remote UI | [07 — Tutorial](07-tutorial-build-a-plugin.md) |
| Add a server-side feature in Go | [15 — Backend plugins](15-backend-plugins.md) |
| Look up a field in `image.json` | [02 — image.json reference](02-image-json.md) |
| Look up an extension API method | [06 — Extension API reference](06-extension-api.md) |
| Look up the Go plugin contract | [15 — Backend plugins](15-backend-plugins.md) |
| Know where you are allowed to render | [05 — Slots](05-slots.md) |
| Know who sees your extension | [08 — Scoping and visibility](08-scoping-and-visibility.md) |
| Make your UI match the app's look | [09 — Styling and icons](09-styling-and-icons.md) |
| Test what you built | [10 — Fixtures](10-fixtures.md), [11 — Testing](11-testing.md) |
| Call the endpoints directly | [12 — HTTP API](12-http-api.md) |
| Understand the trust model | [13 — Security model](13-security-model.md) |
| Fix something that is not working | [14 — Troubleshooting](14-troubleshooting.md) |
| Ship an app to a running server without releasing Remote | [16 — Uploaded packages](16-uploaded-packages.md) |
| Ship a new version of an app people already installed | [17 — Versions and upgrades](17-versions-and-upgrades.md) |

## All documents

1. [Overview](01-overview.md) — the moving parts, and how a request flows through them.
2. [image.json reference](02-image-json.md) — every field, with types and defaults.
3. [Image types](03-image-types.md) — `service`, `ui`, `backend`, and which one creates a container.
4. [Install scripts](04-install-scripts.md) — the contract, the environment, idempotency.
5. [Slots](05-slots.md) — every place an extension may render, and the context each provides.
6. [Extension API reference](06-extension-api.md) — the complete `remote` object.
7. [Tutorial: build a plugin](07-tutorial-build-a-plugin.md) — end to end, from empty directory to working button.
8. [Scoping and visibility](08-scoping-and-visibility.md) — install scope is render scope.
9. [Styling and icons](09-styling-and-icons.md) — design tokens, theming, the icon field.
10. [Fixtures](10-fixtures.md) — `ui-playground`, `ui-sandbox`, `backend-playground`, and how to test with them.
11. [Testing](11-testing.md) — unit tests, the self-test, and browser verification.
12. [HTTP API](12-http-api.md) — every endpoint, its shape, and its authorization.
13. [Security model](13-security-model.md) — what is enforced, where, and what is not.
14. [Troubleshooting](14-troubleshooting.md) — symptoms, causes, fixes.
15. [Backend plugins](15-backend-plugins.md) — shipping Go that runs on the server, and calling it from `ui/`.
16. [Uploaded packages](16-uploaded-packages.md) — the same catalog entry, delivered as a `.zip` at runtime and surviving updates.
17. [Versions and upgrades](17-versions-and-upgrades.md) — how `version` decides when an installed copy is re-provisioned.

## Conventions in these documents

Paths are given relative to the repository root unless stated otherwise. These
documents live in the repository's documentation tree, at
`docs/dev/installable-images/`, while the thing they describe is a directory of
its own at the repository root:

```
images/
```

Keeping the two apart is deliberate: everything under `images/` is embedded
into the server binary by `//go:embed`, and documentation has no business
shipping inside a binary. The directive that embeds it is `catalog.go` at the
repository root, in a small module of its own, because `go:embed` cannot reach
above the directory it is written in.

Code references name the file and, where useful, the symbol —
`registry_ui.go:loadImageUI`, `extensionContributionState.ts:visibleExtensionContributions`.
