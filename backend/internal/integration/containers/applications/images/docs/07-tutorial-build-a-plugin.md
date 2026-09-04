# 07 — Tutorial: build a plugin

We will build a plugin end to end: an image that adds an "Open in Cursor"
button to the chat header, opening the current workspace in the Cursor editor.
It installs nothing in a container, so it is a `type: "ui"` image.

By the end you will have touched every part of the system: the manifest, the
entry module, a view, styling, an icon, and the install-and-test loop.

## 1. Create the directory

From the repository root:

```bash
mkdir -p installable-images/open-in-cursor/ui/{scripts,style,views,assets}
```

The directory name is the image id. Use lowercase letters, digits, and dashes.

## 2. Write `image.json`

```bash
cat > installable-images/open-in-cursor/image.json <<'JSON'
{
  "id": "open-in-cursor",
  "name": "Open in Cursor",
  "description": "Adds a chat-header action that opens the current workspace in Cursor.",
  "category": "development",
  "version": "1",
  "icon": "ui/assets/logo.svg",
  "type": "ui",
  "scopes": ["global", "project"]
}
JSON
```

Notes:

- `id` **must** equal the directory name.
- `type: "ui"` means no container, no port, no install script.
- We omit the `ui` block entirely: the layout convention finds
  `scripts/main.js`, every `.css` under `style/`, and every `.html` under
  `views/`. See [02 — image.json reference](02-image-json.md).
- `icon` points into our own `ui/`, so we ship our own mark.

## 3. Add the icon

```bash
cat > installable-images/open-in-cursor/ui/assets/logo.svg <<'SVG'
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"
     stroke="currentColor" stroke-width="1.6" stroke-linecap="round"
     stroke-linejoin="round">
  <path d="M12 3 21 8v8l-9 5-9-5V8l9-5Z"/>
  <path d="m9 10 3 2 3-2M12 12v5"/>
</svg>
SVG
```

`stroke="currentColor"` makes it follow the theme. See
[09 — Styling and icons](09-styling-and-icons.md).

## 4. Write the entry module

```bash
cat > installable-images/open-in-cursor/ui/scripts/main.js <<'JS'
// Adds a chat-header action that opens the chat's workspace in Cursor.

const CURSOR_ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" ' +
  'stroke-linecap="round" stroke-linejoin="round">' +
  '<path d="M12 3 21 8v8l-9 5-9-5V8l9-5Z"/><path d="m9 10 3 2 3-2M12 12v5"/></svg>';

export default function activate(remote) {
  remote.ui.addIconButton(remote.slots.chatHeaderActions, {
    icon: CURSOR_ICON,
    label: "Open in Cursor",
    title: "Open this workspace in Cursor",
    order: -50,
    // The chat header always carries a cwd, but a guard costs nothing and
    // documents the dependency.
    when: (context) => Boolean(context.cwd),
    onClick: (context) => open(remote, context),
  });
}

function open(remote, context) {
  const url = `cursor://file${context.cwd}`;
  remote.log("opening", url);
  window.open(url, "_blank", "noopener");
}
JS
```

That is a working plugin. Everything below is refinement.

## 5. Add a confirmation popup

Opening a `cursor://` link silently fails if Cursor is not installed, so show
what we tried. Add a view:

```bash
cat > installable-images/open-in-cursor/ui/views/opened.html <<'HTML'
<div class="cursor-ext">
  <p class="cursor-ext__lead">Asked your browser to hand this workspace to Cursor:</p>
  <pre class="cursor-ext__url" data-field="url">…</pre>
  <p class="cursor-ext__hint">
    Nothing happened? Cursor may not be installed, or your browser may be
    blocking the <code>cursor://</code> handler.
  </p>
</div>
HTML
```

and its stylesheet — remember Tailwind is **not** available here, so style with
the platform's CSS custom properties:

```bash
cat > installable-images/open-in-cursor/ui/style/cursor.css <<'CSS'
.cursor-ext__lead,
.cursor-ext__hint {
  margin: 0;
  font-size: 12px;
  line-height: 1.55;
  color: rgb(var(--fg-muted));
}

.cursor-ext__url {
  margin: 8px 0;
  overflow-x: auto;
  border-radius: 6px;
  background: rgb(var(--bg-inset-rgb));
  padding: 8px 10px;
  font-size: 12px;
  color: rgb(var(--fg));
}
CSS
```

Then wire it into the click handler:

```js
function open(remote, context) {
  const url = `cursor://file${context.cwd}`;
  remote.log("opening", url);
  window.open(url, "_blank", "noopener");

  remote.ui.openPopup({
    title: "Open in Cursor",
    width: 460,
    mount: (body) => {
      void remote.views.load("opened").then((html) => {
        body.innerHTML = html;
        const field = body.querySelector('[data-field="url"]');
        if (field) field.textContent = url;
      });
    },
  });
}
```

## 6. Build and run

The catalog is embedded in the binary, so a rebuild is required:

```bash
cd backend && go build ./... && go test ./internal/integration/containers/applications/
```

If `image.json` is malformed or the `ui` block names a missing file, this is
where you find out — the registry validates every entry at load.

Then run the stack:

```bash
cd backend  && go run ./cmd/remote      # terminal 1
cd frontend && npm run dev              # terminal 2
```

## 7. Install it

Nothing appears until the image is installed — that is deliberate, see
[08 — Scoping and visibility](08-scoping-and-visibility.md).

- **Everywhere:** Settings → Applications → Install. Admin only.
- **One project:** that project's settings → Applications → Install.

Because it is a `ui` image, install completes instantly and needs no LXD.

The button appears immediately, without a reload. Open a chat and look at the
header rail.

## 8. Iterate

Editing `ui/` files requires a **backend rebuild**, because the assets are
embedded in the binary — `npm run dev` will not pick them up. The loop is:

```bash
# after editing anything under ui/
cd backend && go build -o /tmp/remote ./cmd/remote && /tmp/remote
```

then reload the browser. The extension's assets are served with
`Cache-Control: private, max-age=300`, so a hard reload (or DevTools with
"Disable cache") avoids a stale module during rapid iteration.

## 9. Check the scope behaviour

Install it **in one project only**, then:

- Open a chat in that project → the button is there.
- Open a chat in another project → it is gone.
- Stop the app on its installed row → it disappears everywhere.
- Start it again → it comes back.

If that is not what you see, [14 — Troubleshooting](14-troubleshooting.md).

## 10. Ship it

```bash
git add installable-images/open-in-cursor
git commit -s -m "feat(applications): add open-in-cursor UI plugin"
```

Follow the repository's [CONTRIBUTING](../../../../../../../CONTRIBUTING.md)
conventions: Conventional Commits with an area scope, DCO sign-off.

Remember that a `ui/` directory is frontend code running on the main origin
with the SPA's privileges — it gets the same review as any other frontend
change here. See [13 — Security model](13-security-model.md).

## Where to go next

- Add the button to more surfaces — [05 — Slots](05-slots.md).
- Read installed apps or project data — [12 — HTTP API](12-http-api.md).
- Compare against a fixture that exercises everything —
  [10 — Fixtures](10-fixtures.md).
- Add an install script so the plugin also provisions something —
  [04 — Install scripts](04-install-scripts.md), and switch `type` to
  `service`.
