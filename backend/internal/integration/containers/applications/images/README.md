# Application images

This is the catalog of one-click installable apps ("Applications" tab). It is
**embedded into the backend binary** (`//go:embed images` in `registry.go`) and
surfaced at the repo root as `installable-images/` (a symlink) so it is
discoverable from the project root.

An image is one directory. It can install software into a container, contribute
to the Remote interface from the browser, or both:

```
images/
  docs/              ← full documentation (reserved name, not an image)
  postgresql/
    image.json       metadata: name, description, port, env, systemd service
    install.sh       idempotent installer, run as root inside the container
  ui-playground/
    image.json
    ui/              browser extension: buttons, panels, popups
      views/*.html
      style/*.css
      scripts/main.js
```

## 📚 Full documentation: [`docs/`](docs/)

Everything is documented in detail there. Start with
[`docs/README.md`](docs/README.md).

| I want to… | Read |
|---|---|
| Understand the system | [Overview](docs/01-overview.md) |
| Add a database or service | [Image types](docs/03-image-types.md), [Install scripts](docs/04-install-scripts.md) |
| Add a button or panel to the UI | [Tutorial](docs/07-tutorial-build-a-plugin.md) |
| Look up an `image.json` field | [image.json reference](docs/02-image-json.md) |
| Look up an extension API method | [Extension API](docs/06-extension-api.md) |
| Know where I can render | [Slots](docs/05-slots.md) |
| Know who sees my extension | [Scoping and visibility](docs/08-scoping-and-visibility.md) |
| Match the app's look | [Styling and icons](docs/09-styling-and-icons.md) |
| Test it | [Fixtures](docs/10-fixtures.md), [Testing](docs/11-testing.md) |
| Call the endpoints | [HTTP API](docs/12-http-api.md) |
| Understand the trust model | [Security model](docs/13-security-model.md) |
| Fix something broken | [Troubleshooting](docs/14-troubleshooting.md) |

## Adding an app, in short

1. Create `images/<id>/image.json`. `id` must equal the directory name.
2. Pick a `type`:
   - `service` — runs software on a port. Add an `install.sh` and a
     `port.internal`. A **global** install gets its own LXD container; a
     **project** install goes into that project's existing container.
   - `ui` — installs nothing anywhere. Add a `ui/` directory; declare no port.
3. Optionally add `ui/` to contribute to the interface. The layout is the
   manifest: `scripts/main.js` is the entry, `style/*.css` are injected,
   `views/*.html` are loadable by name.
4. Rebuild the backend. `NewRegistry()` validates every entry at startup, so a
   malformed image fails the build and the tests rather than 404ing in a
   browser.

No other code changes are required — the app appears in the catalog
automatically for both scopes.

## Two things that surprise people

**Being in the catalog grants nothing.** An extension loads only after a user
installs the image — globally, or in a project they belong to — and only while
that instance is running. Stopping an app turns its UI off. See
[Scoping and visibility](docs/08-scoping-and-visibility.md).

**Extension code is frontend code.** It runs on the main origin with the SPA's
privileges; the trust boundary is the build, not the request. A new `ui/` in a
pull request deserves the same review as any change under `frontend/src`. See
[Security model](docs/13-security-model.md).

## Fixtures

Two `type: "ui"` images exist to exercise this surface, and install anywhere
because they need no container:

- **`ui-playground`** — contributes to every slot with every mechanism, and
  ships an in-app API self-test.
- **`ui-sandbox`** — a second extension sharing those slots, which explains why
  it is visible where it is.

Install them at different scopes to see the whole feature in one pass. See
[Fixtures](docs/10-fixtures.md).
