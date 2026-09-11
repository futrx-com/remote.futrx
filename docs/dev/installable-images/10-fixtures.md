# 10 — Fixtures

> **These fixture images are not in this repository.** What ships here is one
> worked example, [`hello-remote`](../../../images/hello-remote/README.md), which covers the
> same ground more briefly: a slot contribution, a view, a plugin process, and
> per-instance storage. The fixtures below are described as the fuller surface
> a developer working on the extension API itself would want, and the document
> stands as the specification for them.

Three images exist purely to exercise the extension surface. None of them
needs LXD or creates a container, so you can run the whole extension system on
a laptop.

| Fixture | Icon | Type | Purpose |
|---|---|---|---|
| `ui-playground` | flask | `ui` | The full browser surface: every slot, every mechanism, plus an API self-test |
| `ui-sandbox` | cube | `ui` | A second extension sharing the same slots, explaining *why* it is visible where it is |
| `backend-playground` | server | `backend` | The full server surface: a Go plugin exercising every part of the backend contract, and the UI that calls it |

They order themselves `-100`, `-99`, and `-98`, so the set always renders
flask-then-cube-then-server in every shared slot regardless of load order.

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

## Backend Playground

The counterpart to UI Playground on the other side of the wire. It ships a Go
plugin in `plugin/main.go` and a `ui/`
that calls it, and every route exists to demonstrate one property of the
contract:

| Route | Demonstrates |
|---|---|
| `health` | the plugin is a live process — pid, uptime, and a request counter that climbs |
| `echo` | what crosses the boundary, and what does not: no cookies, and a caller the browser cannot forge |
| `instance` | the install the host handed over, redacted by caller — the pattern for anything sensitive |
| `kv` | state in the process, written by one request and read by the next |
| `notes` | state on disk, in the `DataDir` that survives stop and start |
| `compute` | real Go work on the server, which is the reason to have a backend at all |
| `slow` | the image's `timeoutMs`, from the caller's side |
| `boom` | a panic: one failed call, and the same pid afterwards |
| `admin` | a plugin authorizing its own callers, beyond the image's `access` level |

### Where it appears

- A **server icon** in the chat header and the composer, opening the console.
- A **Console** button on its own application card.
- A **panel** under the applications list showing live plugin health and the
  route table the plugin itself advertises.

### The plugin console

The popup runs any of the routes above and appends the raw answer to a log,
newest first, each entry labelled with the call it came from. That labelling
matters: several calls can be in flight at once, they land in completion order
rather than click order, and a shared unlabelled pane would show whichever
finished last and name none of them.

Its buttons are in two groups, because three of them are *supposed* to fail and
a red result from an unmarked button reads as a broken plugin:

| Button | Expected |
|---|---|
| `panic` | fails with the panic message; `health` afterwards shows the same pid |
| `timeout (11s)` | fails after the image's 10s `timeoutMs`; the next call still works |
| `unknown route` | `404` from the plugin's mux |

They are dashed, grouped under "Meant to fail", and their results are logged in
amber and tagged `expected` rather than in red. Watching the pid stay the same
across all three is the whole failure-isolation claim in one screen.

(`admin only` sits with the ordinary routes: it answers JSON for an
administrator and `403` for everyone else, so whether it "fails" depends on who
is asking — which is the point of it.)

### The backend self-test

The panel and the console both have **Run backend self-test**: fourteen checks
asserting the contract from inside a real extension, against a real plugin
process, over the real route.

| Check | What it proves |
|---|---|
| a running backend is available | install gating reaches `remote.backend` |
| describe reports version and routes | the discovery half of the contract |
| health answers from a live process | the plugin started and connected |
| the same process serves consecutive calls | one process per instance, and it is long-lived |
| method, query, and body arrive | the request is forwarded faithfully |
| the caller is stamped by the server | authorization has something to trust |
| the session cookie is withheld | a plugin cannot act as its caller |
| in-memory state survives between calls | the process is not per-request |
| the data directory is writable | `DataDir` works and is the plugin's own |
| real Go work runs on the server | `fib(30)` and a prime sieve, computed host-side |
| the instance is this image | `Init` handed over the right install |
| an unknown route is refused | the mux, and `404` rather than a hang |
| a wrong method is refused | `405` rather than a silent `GET` |
| a panic costs one request, not the process | the pid is unchanged afterwards |

**Run it after changing anything in `pkg/appplugin`, `internal/integration/pluginhost`,
or the backend routes.** It is the browser-side counterpart to
`TestBackendPlaygroundRunsFromTheEmbeddedCatalog`, which asserts the same
things without a browser.

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
4. Settings → Applications → install **Backend Playground** (global). The
   first install compiles it, so it takes a few seconds; every later start is
   instant.

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
| Backend Playground's panel | Live pid and uptime, climbing |
| Stop Backend Playground | Its icons disappear; a console call would report the app is not running |
| Start it again | A **new** pid, and uptime back at zero — the process really was killed |
| Install it in alpha as well | Two entries in `remote.backend.instances`, and alpha's console targets alpha's process |

That table is the whole feature in one pass: install gating, scope gating,
coexistence, ordering, and lifecycle.

### Testing backend plugins without a browser

`backend-playground` is also exercised headlessly, which is what makes it a
regression test rather than only a demo:

```bash
cd backend
go test ./internal/integration/pluginhost/ -run TestBackendPlayground -v
```

That compiles the shipped image from the embedded catalog, runs it, and asserts
the same properties the in-app self-test does. Add `-short` to skip every test
in the package that needs the Go toolchain.

## Should fixtures ship in production?

They are in the catalog like any other image, so they appear in the Applications
tab of every server built from this tree — described as developer fixtures, and
inert until someone installs them.

To drop them from a build, delete the directories. Nothing references them by
id except their own tests:

- `registry_test.go:TestRegistryLoadsDeclaredImageUI` uses `ui-playground` to
  cover the explicit `ui` manifest path.
- `registry_test.go:TestRegistryImageKinds` asserts both playgrounds' `type`.
- `registry_plugin_test.go:TestRegistryPluginSource` and
  `pluginhost/catalog_test.go` use `backend-playground`.

Update those if you remove it.

## Writing your own fixture

If you are adding a slot or an extension API method, extend `ui-playground`
rather than making a fourth fixture; if you are adding to the plugin contract,
extend `backend-playground`. The point of both is to be the one place that
exercises everything. Add:

- a contribution to the new slot, or a route for the new capability, so it is
  visibly covered;
- a self-test check for it, so a regression is caught by clicking one button.
