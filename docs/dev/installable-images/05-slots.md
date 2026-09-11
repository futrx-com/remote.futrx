# 05 — Slots

A slot is a named place in the Remote interface where an extension may render.
They are a **closed set**, defined in
[`frontend/src/config/extensions.ts`](../../../frontend/src/config/extensions.ts).

## Why closed

A slot is a promise: *your code renders here, and receives this context*. That
promise is what keeps an extension working when the surrounding UI is
refactored. Open-ended DOM patching would give you everything on day one and
break silently on every change, with no way for us to know we broke you.

Adding a slot is one `<ExtensionSlot>` line at the render site plus a name in
`config/extensions.ts` — minutes of work. If you need one, ask rather than reaching into
the DOM.

An extension naming a slot that does not exist loses that one contribution and
keeps running, with a console warning. So an image built against a newer Remote
degrades on an older one instead of failing to load.

## The slots

| Constant | Name | Where it renders | Context |
|---|---|---|---|
| `chatHeaderActions` | `chat.header.actions` | Chat header rail, ahead of the IDE / terminal / files / schedules / preview icons | `chatId`, `projectId`, `cwd` |
| `composerActions` | `chat.composer.actions` | Composer control deck, beside the attach (`+`) button | `projectId` |
| `projectRowActions` | `sidebar.project.actions` | A project row's hover actions, ahead of container info and "New chat" | `projectId`, `projectName` |
| `sidebarHeaderActions` | `sidebar.header.actions` | Sidebar header, beside "New project" | — |
| `sidebarSearchActions` | `sidebar.search.actions` | Inside the sidebar search field, trailing | — |
| `applicationCardActions` | `applications.card.actions` | Action row of an installed application's card | `scope`, `projectId`, `instance` |
| `projectSettingsPanel` | `project.settings.panel` | Project settings, above resource limits | `scope`, `projectId` |
| `applicationsPanel` | `applications.panel` | Below the applications list, per scope | `scope`, `projectId` |

Always reference them through `remote.slots.<constant>` rather than typing the
string, so a rename is caught at the API boundary.

## Slot context

Every render function and predicate receives a context object:

```ts
interface ExtensionSlotContext {
  slot: string;          // always set — the slot being rendered
  scope?: "global" | "project";
  instance?: Record<string, unknown>;  // the installed instance, on card slots
  projectId?: string;
  projectName?: string;
  chatId?: string;
  cwd?: string;          // the chat's working directory
}
```

Fields are present only where the surface knows them — read the table above.
The quickest way to see a real one is to install `ui-playground` and click any
flask icon: it prints that slot's exact context. See
[10 — Fixtures](10-fixtures.md).

## The five chrome slots take icons

`chatHeaderActions`, `composerActions`, `projectRowActions`,
`sidebarHeaderActions`, and `sidebarSearchActions` are chrome — rows of small
icon buttons. Use `remote.ui.addIconButton` for them and pass only the mark:
**the slot decides the size**, so your icon matches its neighbours without you
knowing the app's densities.

| Slot | Button | Icon |
|---|---|---|
| `chatHeaderActions` | 32×32 | 16×16 |
| `composerActions` | 32×32 | 16×16 |
| `projectRowActions` | 28×28 | 14×14 |
| `sidebarHeaderActions` | 32×32 | 16×16 |
| `sidebarSearchActions` | 20×20 | 12×12 |

Sizing lives in `SLOT_ICON_APPEARANCE` in `config/extensions.ts`, and a test asserts every
slot has an entry.

## The two application slots

`applicationCardActions` renders once **per installed application**, including
other images'. Scope it to your own with `when`:

```js
remote.ui.addButton(remote.slots.applicationCardActions, {
  label: "Connect",
  when: (context) => context.instance?.imageId === remote.image.id,
  onClick: (context) => { /* context.instance is the installed instance */ },
});
```

Without `when`, your button appears on every application's card, which is
almost never what you want.

`applicationsPanel` renders below the applications list, once per scope. Its
context carries `scope: "global"` on the server-wide Applications settings page
and `scope: "project"` with a `projectId` on a project's own. Note that a
project-installed extension never reaches the global page — see
[08 — Scoping and visibility](08-scoping-and-visibility.md).

## Ordering

Both `register` and the `add*Button` helpers take `order` (ascending, default
`0`). Ties keep registration order, which is catalog order.

Negative numbers put you ahead of the app's own controls. The two fixtures use
`-100` and `-99` so they always render in the same relative order regardless of
which loaded first.

## Rendering

`ExtensionSlot.tsx` gives each contribution its own `<div>` and calls its render
function with that element and the context:

```js
remote.ui.register(remote.slots.applicationsPanel, (host, context) => {
  host.innerHTML = "<p>Hello</p>";
  const timer = setInterval(tick, 1000);
  return () => clearInterval(timer);   // cleanup, called on unmount
});
```

Return a function to register cleanup. It runs when the slot unmounts or its
context changes, and the host element is emptied afterwards. A render or
cleanup that throws is caught and logged — it costs that contribution, nothing
more.

The slot re-reads its contributions whenever the registry changes, so an
extension that loads after the surface has already rendered still appears.
