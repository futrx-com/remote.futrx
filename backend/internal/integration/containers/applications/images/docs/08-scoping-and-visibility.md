# 08 — Scoping and visibility

Three questions decide whether a user sees an extension:

1. Is it **in the build**? (It has to be a directory in the catalog.)
2. Has this user **installed** it, and is it **running**?
3. Is this **surface in scope** for that install?

This document is about 2 and 3.

## Installation is the gate

Shipping an image in the catalog puts nothing on anyone's screen. An extension
loads for a user only when:

- the image is installed **globally**, or installed in a **project that user
  belongs to**, **and**
- that instance's status is **`running`**.

The browser never decides this. It asks `GET /api/applications/ui`, which
returns the list it is allowed to load; the catalog endpoint is not enough and
is not used for this purpose.

Because status matters, **Stop** is a real off switch: stopping an app removes
its UI, and starting it brings it back. Uninstalling drops it entirely. All
three take effect without a page reload — the host re-syncs after every
lifecycle action.

## Install scope is render scope

Where an image is installed decides where its contributions may draw:

| Installed | Renders |
|---|---|
| globally | everywhere — every project, and outside them |
| in project P | only while the user is working inside P; nowhere otherwise |
| globally **and** in P | everywhere (the union wins) |

The extension does not choose this and cannot widen it. The backend reports the
scope of each install, the host records it before the entry module runs, and
`ExtensionRegistry` filters every contribution on every render.

## What "in P" means

A project's extension belongs to **the time the user spends in that project**,
not to anything they can merely see from elsewhere. Two conditions must both
hold for a project install to render:

1. **The user is currently working inside a project that installed it.** This
   is the primary gate, and it applies to every surface. Open another project's
   chat, or no chat at all, and the extension is gone completely — including
   from its own project's sidebar row, which stays on screen the whole time.
2. **Where the surface belongs to a project of its own** — a project row, a
   chat header, a composer, that project's Applications page — it is one of
   those projects. This is what keeps a plugin off *other* projects' rows while
   its own project is open.

**Surfaces that are explicitly global** — the server-wide Applications settings
page — are never reached by a project install, even while that project is
active.

### The active project

The "currently working in" signal is computed in one place,
`WorkspaceContainer.tsx`:

```
the project whose settings are open
  ↓ otherwise
the project owning the open chat
  ↓ otherwise
none
```

and pushed to the registry with `setActiveProject`. With no project active, no
project-installed extension renders anywhere.

## The rule, exactly

From `extensionContributionState.ts:isInScope`:

```
if (install.global)             → visible
if (context.scope === "global") → hidden
if (activeProjectId is null or not in install.projectIds) → hidden
if (context.projectId is set and not in install.projectIds) → hidden
→ visible
```

Then, and only then, the contribution's own `when` predicate runs. `when` is
the extension's choice; scope is not.

## Worked example

`ui-playground` installed globally, `ui-sandbox` installed in project *alpha*:

| Where the user is | Playground (global) | Sandbox (alpha only) |
|---|---|---|
| No chat open | sidebar header, search, both project rows | **nothing** |
| A chat in **alpha** | all five chrome slots | all five chrome slots |
| A chat in **beta** | all five chrome slots | **nothing** |
| Server-wide Applications page | its panel | nothing |
| Alpha's Applications page | its panel | its panel |

The sandbox is not merely absent from beta's chat header — it is absent from
alpha's own sidebar row too, for as long as the user is reading beta. Leaving a
project puts its plugins away entirely.

## Multiple installs of one image

The same image can be installed globally and in several projects at once. The
backend unions them into one entry:

```json
{ "image": { "id": "my-plugin", … }, "global": true, "projectIds": ["p1", "p2"] }
```

The extension is loaded **once** and its contributions are scoped to that
union. `remote.install` reports the same record, so an extension can describe
its own reach:

```js
function describeInstall(remote) {
  const { global, projectIds } = remote.install;
  if (global) return "globally — visible everywhere";
  if (!projectIds.length) return "nowhere";
  return `in ${projectIds.length} project(s) — visible only there`;
}
```

## Membership

`GET /api/applications/ui` resolves the caller's visible projects with the
project service's own `ListVisible(email, isAdmin)`, so extension visibility
inherits project membership exactly. A project you are not a member of never
enters the list, so its extensions can never reach you — the frontend filter
never even sees them.

## Changing scope mid-session

If an image goes from global to project-only (or the reverse) while the user is
signed in, the next sync re-records its visibility and **re-stamps
contributions already registered**. The extension is not reloaded; its existing
buttons simply narrow or widen. This is covered by
`extensionStore.test.ts:"re-recording visibility re-scopes contributions
already registered"`.

## Testing the rule

The behaviour is pinned by:

- `extensionStore.test.ts` — nine tests covering global, project, other
  project, no project, a project extension disappearing entirely while the user
  works elsewhere, the explicitly-global surface, the union case, re-scoping,
  and two extensions ordering in one slot.
- `service/applications/ui_extensions_test.go` — the backend's aggregation:
  install scope reporting, the union of several installs, the membership
  boundary, and skipping stopped or UI-less images.

To check it by hand, install the two fixtures at different scopes — see
[10 — Fixtures](10-fixtures.md).
