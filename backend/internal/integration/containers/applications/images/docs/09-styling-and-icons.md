# 09 — Styling and icons

## Tailwind is not available in extension code

Extension assets are served from the catalog, not built with the SPA. Tailwind
only emits classes it finds while scanning `frontend/src`, so a class you write
in `ui/style/*.css` or `ui/scripts/*.js` **does not exist** in the stylesheet
the browser loads.

Write ordinary CSS in your own stylesheet, and style with the platform's CSS
custom properties.

## Design tokens

The tokens are defined on `:root` in `frontend/src/index.css` and flip with the
light/dark theme, so using them themes your UI for free.

Colours come in two flavours:

**Channel triplets** — `--fg: 227 227 231`. Use through `rgb()`, which lets you
add alpha:

```css
color: rgb(var(--fg));
background: rgb(var(--accent-blue) / 0.14);
```

**Complete values** — `--line: rgba(255, 255, 255, 0.07)`. Use directly:

```css
border: 1px solid var(--line);
```

### Text

| Token | Use |
|---|---|
| `--fg-bright` | headings, emphasised content |
| `--fg` | body text |
| `--fg-dim` | secondary content |
| `--fg-muted` | supporting text |
| `--fg-subtle` | labels, captions |
| `--fg-faint` | disabled text |

### Surfaces

Deepest to highest: `--bg-app-rgb`, `--bg-canvas-rgb`, `--bg-surface-rgb`,
`--bg-raised-rgb`, `--bg-inset-rgb`.

A panel sits on `--bg-surface-rgb`; a code block or input well uses
`--bg-inset-rgb`.

### Lines and accents

`--line` for hairlines. Accents: `--accent-blue`, `--accent-green`,
`--accent-red`, `--accent-yellow`, `--accent-orange`, `--accent-purple` — all
channel triplets.

Use `--accent-red` for destructive, `--accent-green` for healthy,
`--accent-blue` for the primary action. Pick one accent as your extension's
identity: the two fixtures are neutral and purple respectively, which makes it
obvious at a glance which drew what.

## A panel that looks native

```css
.my-panel {
  border: 1px solid var(--line);
  border-radius: 10px;
  background: rgb(var(--bg-surface-rgb));
  padding: 12px 14px;
}

.my-panel__title {
  margin: 0;
  font-size: 13px;
  font-weight: 500;
  color: rgb(var(--fg-bright));
}

.my-panel__lead {
  margin: 6px 0 0;
  font-size: 11.5px;
  line-height: 1.55;
  color: rgb(var(--fg-muted));
}

.my-panel__code {
  margin: 8px 0 0;
  overflow-x: auto;
  border-radius: 6px;
  background: rgb(var(--bg-inset-rgb));
  padding: 8px 10px;
  font-size: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  color: rgb(var(--fg));
}
```

The app's type scale is small: 13px for titles, 12–12.5px for body, 11.5px for
supporting text, 11px for captions. Matching it matters more than any colour
choice for looking native.

## Namespace your class names

Your stylesheet is injected globally into the document — there is no scoping.
Prefix every class with something unique to your image (`.mysql-ext-`, `.pg-`,
`.sb-`) so two extensions cannot collide, and so you cannot accidentally
restyle the app.

The flip side: because the stylesheet is global, it *can* target the app's own
markup. The app has stable semantic hooks — `.app-shell`, `.codex-sidebar`,
`.codex-main`, `.codex-window-frame` — but most of its classes are Tailwind
utilities that change whenever that markup is touched. Restyling the app is
possible and unsupported; assume it will break.

## Stylesheet lifecycle

Stylesheets are injected as `<link>` elements when an extension loads. When an
extension is uninstalled or stopped, **the stylesheet stays in the document** —
CSS cannot be reliably unapplied mid-session — but all of its contributions are
removed, so nothing it styles is rendered any more. Namespaced class names make
this a non-issue.

## The `icon` field

`icon` in `image.json` takes either form:

```json
"icon": "database"              // a built-in key
"icon": "ui/assets/logo.svg"    // a file this image ships
```

### Built-in keys

`activity`, `archive`, `bell`, `bot`, `box`, `cache`, `clock`, `code`, `cpu`,
`database`, `disk`, `folder`, `key`, `layers`, `memory`, `monitor`, `network`,
`server`, `settings`, `shield`, `terminal`, `users`.

Anything unrecognised falls back to a generic server mark. The mapping is
`BUILT_IN` in
[`frontend/src/ui/applications/AppIcon.tsx`](../../../../../../../frontend/src/ui/applications/AppIcon.tsx).

### Shipping your own

A value starting with `ui/` is a path into your image's own `ui/` directory,
served through the same authenticated asset route as the rest of the extension.
Any format the browser renders works.

An image may ship a `ui/` that contains **nothing but an icon** — it does not
need an entry module, styles, or views to use this.

For an SVG, use `fill="none" stroke="currentColor"` (or
`fill="currentColor"`) so the mark follows the surrounding text colour and the
theme. `viewBox="0 0 24 24"` matches the app's own icons.

## Icons in slots

Icons passed to `remote.ui.addIconButton` are sized by the slot, not by you:

| Slot | Button | Icon |
|---|---|---|
| chat header, composer, sidebar header | 32×32 | 16×16 |
| project row | 28×28 | 14×14 |
| sidebar search | 20×20 | 12×12 |

So:

- **Omit `width` and `height`** on the SVG; the host stretches it to the slot's
  size.
- Use `stroke="currentColor"` so hover states work — the button changes text
  colour on hover, and the icon follows.
- Keep `stroke-width` around `1.7`–`1.8` to match the app's icon weight at
  these sizes.

```js
const ICON =
  '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
  'stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">' +
  '<path d="M12 5v14M5 12h14"/></svg>';
```

## Popups

`remote.ui.openPopup` supplies the chrome — backdrop, panel, border, title row,
close button — using the app's tokens. Style only the **body** you fill, and
keep it to the app's type scale. `width` sets the body's max width in pixels
and is clamped to the viewport, so a popup stays usable on a phone.
