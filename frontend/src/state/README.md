# state/

Three folders, each answering one question.

| Folder | Its one job | Holds state? |
| --- | --- | --- |
| `stores/<domain>/` | Global Zustand state, actions, and lifecycle | **Yes** — this is the whole list |
| `hooks/<domain>/` | The access layer: how UI reads a store, plus state owned by one screen | In preact hooks |
| `context/` | Cross-cutting state, gated in nesting order | In preact hooks |

`stores/` and `hooks/` both group by domain, in the same words `ui/` and
`services/` use: **agents, chat, media, push, workspace**. The store-to-store
edges all sit inside one folder — the capability catalog and its wiring, both
push stores and `pushPageFocus`, and the workspace feed — so the folders are
where the coupling already was, not a grid imposed over it.

`media/` is the one domain with no matching `hooks/` folder.
`mediaViewerStore` is opened from file-manager rows and from chat links alike,
and filing it under whichever domain holds today's callers would claim an
ownership it does not have.

`hooks/shared/` is the pressure valve, the way `platform/` is for services: a
hook that knows nothing about any one domain goes there rather than being filed
under whichever domain needed it first. `useShortcut` binds a chord to the
window and `useDismissShortcut` puts one dismissible surface in front of the
rest, and both are called from chat, workspace, projects and `ui/` alike. Keep
it to that -- a hook about a domain has a domain folder.

## Pure functions are not a layer

There used to be a fourth folder, `logic/`, holding the projectors, policies
and reducers. It answered no question of its own: half of it was the private
insides of one hook or store, and half was pure helpers that `ui/` imported
directly. Neither half is a layer, so there is no folder for them now.

**A module lives with its owner.** `chatEventStateProjector` belongs to
`useChat` and sits in `hooks/chat/`. `createProjectForm` is validation for one
modal and lives in `ui/projects/` next to it. Splitting a rule out of the thing
that owns it is still worth doing — it is what makes the rule testable — but
the split is a file, not a directory.

**A module with owners in more than one layer goes to `services/`.**
`usageFormatService` is read by four components, `workspaceSidebarService` by
`app/`, `ui/`, two hooks and a context. They own no state and answer to no
single caller, which makes them leaves — the same category as `config/` and
`models/`. Each one is a class with a single exported instance; see
[`../services/README.md`](../services/README.md).

## The access rule

Every global store is an independent vanilla Zustand store with a flat state
and action surface:

```ts
createStore<DomainStoreState & DomainStoreActions>()((set, get) => ({
  value: initialValue,
  update: (value) => set({ value }),
}));
```

Store modules import `createStore` directly from `zustand/vanilla`; there is no
application store wrapper or shared subscription engine. State transitions stay
inside the declared domain actions and use the initializer's `set` and `get`.
Callers use `getState()` to dispatch those actions, not Zustand's public
`setState`. `storeArchitecture.test.ts` enforces direct vanilla Zustand stores
and keeps store models and fixed configuration in their existing layers.

**Global state is read only through `hooks/`.** Nothing in `ui/` or `app/` may
subscribe to or read from a store — a store outlives the component tree, so a
direct read misses every later change and never re-renders. Hooks select narrow,
stable values with Zustand's `useStore`; `hooks/` and `context/` are the only
reactive importers of `stores/`.

This is about *global* state, not all state. Roughly half the hooks here own
something local — a date range, a textarea's height, a drag in progress — and
those should stay local. Promoting a form's fields to a store to keep the
folder count tidy is the failure this rule exists to prevent.

**A store holds the input, not the result.** Workspace search keeps its
selection — the keyword, the filters, the sort — in `workspaceSearchStore`,
because it outlives the surface that set it and the sidebar's copy is written
back to storage. The index and the ranked hits are not in any store: they are a
function of that selection and of the chats the feed is pushing, so
`useWorkspaceSearch` derives them where both are in hand rather than mirroring
them into state that could fall behind either input.

**Commands may be dispatched from anywhere.** Writing to a store is not a
subscription and carries no re-render obligation. This matters because some
dispatch sites are not components and cannot call a hook — see
`ui/chat/markdown/inlineParser.tsx`, which opens the media viewer from a link
handler inside a vnode builder. That is the one file outside `hooks/` and
`context/` that imports a store, and it is deliberate.

## Naming

- `*Store` — a vanilla Zustand store created through `createStore`. If you add
  one, it belongs in `stores/`.
- `create*Store` — a factory for a store that needs an injected boundary or an
  isolated instance in tests.
- `*State` — keeps nothing; a policy, reducer, or projection over its arguments.

The suffix carries the placement rule, so keep it honest. `promptQueueState`
sits in `hooks/chat/` because it holds nothing; `pushPresenceStore` sits in
`stores/push/` because it holds the chat this client has claimed — global state
that never renders is still global state.

One file in `stores/` carries neither suffix, and it is honest about it.
`stores/push/pushPageFocus.ts` is a three-line read of `document`, private to
the two push stores that call it. Store factories and their application-wide
instances stay together so each domain has one obvious composition point.

## Where the types and the constants live

**Not here.** A data shape belongs in `models/` and a tunable belongs in
`config/`, whichever module happens to compute or consume it. `ChatRenderState`
is declared beside `ChatMeta`, not inside the projector that builds it; the
agent-browser poll interval sits in `config/agents.ts`, not in the hook that
passes it to `setTimeout`.

**Stores have no local-type exception.** Store state/actions, persistence
shapes, listener signatures, request options, and injected boundary contracts
belong in the existing domain model files. Stores combine their domain state
and action contracts when creating the Zustand store; they do not redeclare or
re-export them.

Named fixed defaults belong in `config/` too: the empty workspace snapshot is
in `config/workspace.ts`, and the empty capability catalog snapshot is in
`config/agents.ts`. Runtime state is different: store instances, per-instance
maps, timers, subscriptions, and generated client IDs stay with their owner.

Hooks and contexts may still own their local behavioural contracts, such as
`useUsageDashboard`'s return type or `ConfirmOptions` with its preact children
and action callback. This store boundary does not move those unrelated types.

## Where the tests are, and why

Beside the module they test, which now means throughout `state/` rather than in
one folder. `hooks/` and `context/` still have no test for a hook or a
component — there is no harness in this repo, so the compiler and the build are
the only net for those.

That is still the reason to pull a rule out of a hook: `promptQueueState.ts`
has a test and `usePromptQueue.ts` cannot. When a hook grows a fallback, a
convergence condition, or a mapping worth pinning, move the rule into its own
file and let the hook keep the lifecycle. The file lands next to the hook, so
the move costs nothing but the import.

## Layering

`ui → app → state → api → transport`, with `config`, `models` and `services`
as leaves anyone may import. `state/` imports nothing from `ui/`.
