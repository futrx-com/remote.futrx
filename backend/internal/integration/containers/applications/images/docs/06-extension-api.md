# 06 — Extension API reference

The `remote` object is what an image's entry module receives. It is the public
surface a plugin author writes against, defined in
[`frontend/src/app/extensions/extensionApi.ts`](../../../../../../../frontend/src/app/extensions/extensionApi.ts).

## The entry module

`ui/scripts/main.js` (or whatever `ui.entry` names) is imported once per
session, after sign-in, and its **default export** is called with the API:

```js
export default function activate(remote) {
  // register contributions here
}
```

A named `activate` export is accepted as a fallback. The function may be
`async`; the host awaits it. If it throws, everything it registered is dropped
and the failure is logged — the rest of the interface is unaffected.

It runs **once**, not per navigation. Do not put per-view logic in it; put it
in a slot's render function, which runs whenever that surface mounts.

## `remote` at a glance

```js
remote.apiVersion              // 1
remote.image                   // { id, name, version, icon }
remote.install                 // { global, projectIds }
remote.slots                   // { chatHeaderActions: "chat.header.actions", … }

remote.ui.register(slot, render, options?)      → dispose
remote.ui.addButton(slot, button)               → dispose
remote.ui.addIconButton(slot, button)           → dispose
remote.ui.openPopup(options?)                   → { body, close }

remote.views.load(name)        // Promise<string>
remote.views.url(name)         // string | null
remote.assets.url(path)        // string

remote.backend.available                        // boolean
remote.backend.instances                        // [{ instanceId, scope, projectId }]
remote.backend.call(path, options?)             → Promise<parsed JSON>
remote.backend.fetch(path, options?)            → Promise<Response>
remote.backend.describe(target?)                → Promise<descriptor>
remote.backend.url(path, target?)               // string

remote.log(...args)
```

---

## `remote.apiVersion`

Integer, currently `1`. Bumped only on a breaking change to this object. Guard
against a newer or older host if you care:

```js
if (remote.apiVersion !== 1) return;
```

## `remote.image`

`{ id, name, version, icon }` — the catalog entry this code was loaded from.
`remote.image.id` is the value to compare against `context.instance?.imageId`.

## `remote.install`

Where this copy was installed, and therefore where it renders:

```ts
{ global: boolean, projectIds: string[] }
```

**Read-only, and not advisory.** The registry enforces scope on every render
regardless of what your code does; this field exists so an extension can
*explain* itself, not widen its reach. See
[08 — Scoping and visibility](08-scoping-and-visibility.md).

## `remote.slots`

The slot-name constants. Always use these rather than string literals — see
[05 — Slots](05-slots.md) for the full table.

---

## `remote.ui.register(slot, render, options?)`

The general form. `render` draws into a plain DOM element:

```js
const dispose = remote.ui.register(
  remote.slots.applicationsPanel,
  (host, context) => {
    host.innerHTML = "<p>Hello from my plugin</p>";
    const timer = setInterval(() => {}, 1000);
    return () => clearInterval(timer);
  },
  { order: 10, when: (context) => context.scope === "project" }
);
```

| Parameter | Notes |
|---|---|
| `slot` | A value from `remote.slots`. An unknown name logs a warning and is dropped. |
| `render(host, context)` | `host` is an empty `<div>` you own. Return a cleanup function, or nothing. |
| `options.order` | Ascending; default `0`. Ties keep registration order. |
| `options.when(context)` | Return `false` to skip this contribution for that context. |

Returns a disposer. Calling it removes the contribution; calling it twice is
safe.

## `remote.ui.addButton(slot, button)`

A labelled button in the app's style. Best for the application-card slot.

```js
remote.ui.addButton(remote.slots.applicationCardActions, {
  label: "Connect",
  title: "Show connection details",       // tooltip; optional
  icon: '<svg …>…</svg>',                 // optional, rendered before the label
  variant: "solid",                       // "ghost" (default) | "solid"
  order: -10,
  when: (context) => context.instance?.imageId === remote.image.id,
  onClick: (context) => { … },
});
```

`ghost` blends into surrounding chrome; `solid` reads as a primary action.

## `remote.ui.addIconButton(slot, button)`

An icon-only button for the app's chrome. **The slot decides the size** — pass
the mark and an accessible name, not dimensions.

```js
remote.ui.addIconButton(remote.slots.chatHeaderActions, {
  icon: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" ' +
        'stroke-width="1.8"><path d="M12 5v14M5 12h14"/></svg>',
  label: "Open in Cursor",     // required: aria-label, and the tooltip default
  title: "Open this workspace in Cursor",   // optional tooltip override
  order: -100,
  when: (context) => Boolean(context.cwd),
  onClick: (context) => window.open(`cursor://${context.cwd}`),
});
```

Rules for the icon SVG:

- **Omit `width` and `height`** — the slot sizes it.
- Use `stroke="currentColor"` (or `fill="currentColor"`) so it follows the
  surface's hover and theme colours.
- `viewBox="0 0 24 24"` matches the app's own icons.

`label` is required and is not rendered: it is the button's accessible name, so
write it as one ("Open in Cursor", not "cursor").

## `remote.ui.openPopup(options?)`

A modal with the app's chrome — border, backdrop, close button, Escape and
backdrop-click to dismiss.

```js
const handle = remote.ui.openPopup({
  title: "Connection",
  width: 460,                     // body max width in px; clamped to viewport
  html: "<p>Static markup</p>",   // optional
  mount: (body) => {              // optional; body is the content element
    void remote.views.load("popup").then((html) => {
      body.innerHTML = html;
      body.querySelector("[data-action]")?.addEventListener("click", …);
    });
    return () => { /* optional cleanup on close */ };
  },
});

handle.body    // the content element
handle.close() // close it yourself
```

Use `mount` whenever the content needs wiring; use `html` for static markup.

## `remote.views.load(name)`

Fetches a view declared in the `ui` block and resolves to its HTML as a string.
Rejects if the name is not declared or the request fails.

```js
const html = await remote.views.load("panel");
host.innerHTML = html;
```

Views are plain HTML fragments — no template language. Fill them by querying
the result:

```js
host.querySelector('[data-field="count"]').textContent = String(n);
```

## `remote.views.url(name)`

The URL of a declared view, or `null` if the name is unknown. Useful for an
`<iframe>` or a manual `fetch`.

## `remote.assets.url(path)`

The URL of **any** file under the image's `ui/`, for images, fonts, or data:

```js
img.src = remote.assets.url("assets/logo.svg");
```

`path` is relative to `ui/`. Paths that escape it are refused by the server
with a 404 — see [13 — Security model](13-security-model.md).

---

## `remote.backend`

The image's own Go plugin — the `plugin/` directory beside your `ui/`. It is
present on every extension; images that ship no plugin simply report
`available: false`. The contract a plugin implements is
[15 — Backend plugins](15-backend-plugins.md); this is the browser side of it.

### `remote.backend.available`

`false` when the image ships no `plugin/`, or when none of its installs is
running. Degrade around it rather than letting every call throw:

```js
if (!remote.backend.available) {
  host.textContent = "Start this app to use its backend.";
  return;
}
```

### `remote.backend.instances`

`[{ instanceId, scope, projectId }]` — the running plugins this extension may
call. An image installed globally **and** in two projects runs three processes,
which is why a call has to resolve to one of them.

### Choosing which one a call reaches

Every method takes an optional target:

| Target | Reaches |
|---|---|
| *(nothing)* | the global install, or the only one if there is no global |
| `{ projectId }` | that project's install, falling back to the global one |
| `{ instanceId }` | exactly that install; an unknown id throws |

`projectId` is a **preference, not a filter**, which is what makes the common
case a one-liner: pass the slot's context and a call follows the surface the
user is on, exactly as the extension's own visibility does.

```js
remote.ui.register(remote.slots.applicationsPanel, async (host, context) => {
  const health = await remote.backend.call("health", { projectId: context.projectId });
  host.textContent = `plugin pid ${health.pid}`;
});
```

### `remote.backend.call(path, options?)`

Calls a route and resolves its parsed JSON body. Rejects with the plugin's own
error message when the plugin answers with a non-2xx status.

```js
const written = await remote.backend.call("kv/greeting", {
  method: "POST",                       // defaults to POST when a body is given
  body: { value: "hello" },             // JSON-encoded unless already a string
  query: { dryRun: false },
  headers: { "X-Request-Id": id },
  signal: controller.signal,
  projectId: context.projectId,
});
```

`path` is relative to the instance's `/backend/` prefix and matches the route
the plugin declared — `"health"`, `"kv/greeting"`. Separators survive
encoding; segments are escaped.

### `remote.backend.fetch(path, options?)`

The same call, resolving the raw `Response`. Use it for a non-JSON body, a
stream, or when you want the status rather than an exception.

### `remote.backend.describe(target?)`

What the plugin says about itself, including the routes it serves — useful for
building UI from the plugin rather than duplicating its route list:

```js
const { descriptor, access, timeoutMs } = await remote.backend.describe();
for (const route of descriptor.routes ?? []) {
  console.log(route.method, route.path, route.description);
}
```

### `remote.backend.url(path, target?)`

The URL a call would use, for an `<iframe>`, a download link, or your own
`fetch`. Throws if no install can be resolved.

### What the plugin sees

Not what you send. The server stamps the signed-in caller onto every request
and **withholds your cookies** from the plugin, so a plugin can authorize a
caller but cannot act as them. See
[13 — Security model](13-security-model.md#backend-plugins).

## `remote.log(...args)`

`console.info` tagged with the image id:

```js
remote.log("activated", remote.install.global ? "globally" : "per project");
// [extension:my-plugin] activated globally
```

---

## Error handling

Every boundary is guarded, and every failure is local to one extension:

| What throws | What happens |
|---|---|
| The entry module | Everything it registered is dropped; it is not retried this session |
| A render function | That contribution's node stays empty; the surface renders |
| A cleanup function | Logged; the host element is emptied anyway |
| A `when` predicate | That contribution is hidden; others are unaffected |
| A click handler | Logged; other handlers keep working |
| `views.load` on an unknown name | The returned promise rejects |

Nothing an extension does can take down the SPA — but note this is about
*robustness*, not *security*. Extension code runs with the SPA's privileges; see
[13 — Security model](13-security-model.md).

## What the API deliberately does not offer

- **Arbitrary DOM access to the app.** You can reach `document` — it is the
  same origin — but nothing outside your host elements is supported, and Preact
  will overwrite your changes on the next render.
- **Replacing or reordering existing UI.** Slots add; they do not substitute.
- **A browser-side storage API.** Use `localStorage` under a key prefixed with
  your image id, the project secrets API through `fetch`, or — for anything
  that should live on the server — your image's own plugin, which is given a
  private directory on the host.
- **Cross-extension messaging.** Two extensions share the page and can find
  each other through the DOM, but nothing is provided or supported.

## Calling the Remote API

There is no wrapper: use `fetch` with `credentials: "same-origin"` and you are
the signed-in user, subject to exactly their authorization.

```js
const response = await fetch(
  `/api/projects/${encodeURIComponent(context.projectId)}/applications`,
  { credentials: "same-origin" }
);
```

See [12 — HTTP API](12-http-api.md) for the applications endpoints.
