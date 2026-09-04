# 10 — Fixtures

Two images exist purely to exercise the extension surface. Both are
`type: "ui"`, so they install instantly, need no LXD, and create no containers
— you can run the whole extension system on a laptop.

| Fixture | Icon | Purpose |
|---|---|---|
| [`ui-playground`](../ui-playground/) | flask | The full surface: every slot, every mechanism, plus an API self-test |
| [`ui-sandbox`](../ui-sandbox/) | cube | A second extension sharing the same slots, explaining *why* it is visible where it is |

They order themselves `-100` and `-99`, so the pair always renders
flask-then-cube in every slot regardless of which loaded first.

## UI Playground

Contributes to **every** slot using **every** mechanism:

| Contribution | Mechanism | What it demonstrates |
|---|---|---|
| A flask in all five chrome slots | `addIconButton` | The slot renders, and sizes the icon to its surface |
| "Self-test" on its own cards | `addButton` + `when` | Labelled buttons, and the predicate that keeps a contribution off other images' cards |
| A panel under the applications list | `register` | Custom markup, asset URLs, and cleanup on unmount |

### Inspecting a slot's context

Click **any flask icon** and it prints the exact context that slot passed the
handler:

```json
{
  "slot": "chat.header.actions",
  "chatId": "b42593d84e73",
  "projectId": "0b12e39ee03b",
  "cwd": "/var/lib/remote/projects/alpha/workspace"
}
```

This is the fastest way to learn what a slot gives you before writing against
it — faster than reading [05 — Slots](05-slots.md), and guaranteed current.

### The API self-test

The panel has a **Run API self-test** button (also on the fixture's own
application card) that asserts the API contract from inside a real extension
and reports pass/fail per check:

| Check | What it proves |
|---|---|
| `apiVersion` is a positive integer | the version contract exists |
| image identity is this image | `remote.image` is correct |
| every advertised slot has a name | `remote.slots` is populated |
| `views.url` resolves declared views only | declared names work, unknown ones return `null` |
| `views.load` fetches a declared view | the asset route serves views |
| `views.load` rejects an unknown view | failures reject rather than resolving empty |
| `assets.url` serves a non-view asset | arbitrary `ui/` files are reachable |
| assets outside `ui/` are refused | path traversal is blocked (expects a 404) |
| an unknown slot is dropped, not thrown | forward compatibility |
| a disposer is idempotent | cleanup is safe to repeat |

Two of these deliberately produce console noise — a 404 and an "unknown slot"
warning. That is the assertion working, not a bug.

**Run it after changing the extension API.** Install nothing else, open
Settings → Applications, click the button: ten checks, and you know whether you
broke the contract.

## UI Sandbox

Deliberately smaller. It contributes a cube icon to the same five chrome slots,
a "Scope" button on its own card, and a small panel — and every one of them
opens a popup that explains **why it is visible on the surface you clicked**:

```
Extension        UI Sandbox (ui-sandbox)
Installed        in 1 project — visible only there
Visible because  this surface belongs to project 20336ed6ab63
```

It exists for two things: checking that two extensions share slots cleanly, and
making the scoping rule visible.

## The recommended test setup

Install them at **different scopes**:

1. Settings → Applications → install **UI Playground** (global).
2. Create two projects, *alpha* and *beta*, with a chat in each.
3. Alpha's settings → Applications → install **UI Sandbox** (project scope).

Then:

| Where | Expected |
|---|---|
| A chat in **alpha** | Flask **and** cube in the sidebar header, search field, chat header rail, and composer |
| A chat in **beta** | Flask only — the cube is gone from every surface, including alpha's own sidebar row |
| No chat open | Flask everywhere; no cube anywhere |
| Server-wide Applications page | Playground's panel only |
| Alpha's Applications page | Both panels |
| Stop UI Playground | Every flask disappears, immediately, without a reload |
| Start it again | They come back |

That table is the whole feature in one pass: install gating, scope gating,
coexistence, ordering, and lifecycle.

## Should fixtures ship in production?

They are in the catalog like any other image, so they appear in the Applications
tab of every server built from this tree — described as developer fixtures, and
inert until someone installs them.

To drop them from a build, delete the directories. Nothing references them by
id except their own tests:

- `registry_test.go:TestRegistryLoadsDeclaredImageUI` uses `ui-playground` to
  cover the explicit `ui` manifest path.
- `registry_test.go:TestRegistryImageKinds` asserts its `type`.

Update those if you remove it.

## Writing your own fixture

If you are adding a slot or an API method, extend `ui-playground` rather than
making a third fixture — the point of it is to be the one place that exercises
everything. Add:

- a contribution to the new slot, so it is visibly covered;
- a self-test check for the new method, so a regression is caught by clicking
  one button.
