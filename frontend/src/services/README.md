# services/

Logic that belongs to no single caller, grouped by the domain it serves.

| Folder | What it answers about |
| --- | --- |
| `auth/` | Which agent providers are logged in, and what that gates |
| `chat/` | Where an attachment is stored and what it is called |
| `files/` | What a filename means: its kind, its icon, what a click does |
| `projects/` | The `<slug>--<port>.dev.<host>` preview URL shape |
| `usage/` | Date ranges, bar geometry, and how tokens and money are written |
| `workspace/` | The sidebar: what it shows, and what the user folded away |
| `platform/` | The browser and the language — storage, ids, time, diff |

The domain names are the ones the app already uses in `state/hooks/` and
`ui/`, so a service sits under the same word as the hook and the screen that
call it. `platform/` is the exception and the pressure valve: a module that
knows nothing about this app goes there rather than being filed under
whichever domain happened to need it first.

Every file has the same shape: **one class, one exported instance.**

```ts
// services/projects/projectPreviewUrlService.ts
class ProjectPreviewUrlService {
  build(slug: string, port: number | null, publicHostname: string): string { … }
  private hostSuffix(publicHostname: string): string { … }
}

export const projectPreviewUrlService = new ProjectPreviewUrlService();
```

The class is not exported — only the instance is. Nothing here constructs a
second one, so the type is an implementation detail and the constant is the
whole public surface.

## Why a class and not a module of functions

Because these modules already *were* classes; they were just written as
modules. `projectPreviewUrls` had four exported functions over three private
helpers. `fileMeta` had five over two lookup tables. `usageRangeState` had
eight over one `DAY_MS`. In every case the shared data and the private steps
were held together by nothing but file boundaries, and the exported names
carried a prefix — `usageRange…`, `agentAuth…`, `projectPreview…` — standing
in for the receiver they did not have.

A class makes that grouping real: the tables and constants are private fields,
the intermediate steps are private methods a caller cannot reach, and the
prefix drops off because `usageRangeService.forPreset(…)` already says it.

## The leaf rule

**A service may not import from `ui/`, `app/`, `state/`, `api/` or
`transport/`.** It may import `models/`, `config/`, and other services.

This is the rule that keeps the folder from rotting, and it is worth being
blunt about because the name invites the opposite. In a backend a "service"
orchestrates repositories and clients; here it does no I/O beyond the browser
storage one, and it never fetches. Anything that talks to the server is an
`api/` call, and logic that owns state or a component's lifecycle belongs in
`state/`. A service that grew a fetch would put `services/` in competition with
`state/hooks/` for the same job — so it does not get to grow one.

Being a leaf is what earns the layer its reach: everything above may import it,
because it can never import back.

## What lives here, and what does not

A module belongs here when **no single caller owns it**. `fileService` is read
by the file tree, the markdown parser, the IDE links and a hook;
`workspaceSidebarService` by a container and a context. That is also why
`files/` is its own folder rather than living under `chat/` where all four of
today's callers happen to sit: the service is about files, and the next caller
will not be a chat one.

A module with exactly one owner stays with that owner — see the note in
[`../state/README.md`](../state/README.md). `chatEventStateProjector` sits in
`state/hooks/chat/` and `createProjectForm` in `ui/projects/`, and neither is
any less testable for it.

The two exceptions are `platform/diffService` and
`platform/relativeTimeService`, which have one caller each today. They are
here because they are general capabilities with private internals — a Myers
diff and a pair of time formatters — rather than rules about one screen, and a
second caller would not move them.

## Naming

`*Service`, and the suffix carries the placement rule the way `*Store` does in
`state/stores/`. If you write a class named `…Service`, it goes here; if it
does not belong here, it is not a service.

The filename keeps the domain word its folder already says —
`usage/usageRangeService.ts`, not `usage/rangeService.ts` — because the import
lands in a file where the folder is no longer visible, and `usageRangeService`
reads at the call site where `rangeService` would not.

Methods drop the prefix the receiver now carries. Where two methods differ in
a way a reader could mistake for duplication, the names say so:
`fileService.formatBytes` and `fileService.formatBytesCompact` produce
different strings on purpose, and a test pins both.

## Types, constants and tests

Same rules as everywhere else in the app: a data shape goes to `models/` and a
tunable to `config/`, whichever service happens to compute or consume it.
`FileCategory` sits in `models/files.ts` because the file tree keys its icon
table by it.

What stays is what describes one service's own insides and never appears in an
import elsewhere — `LineDiffPart`, `FileOpenAction`, `ChatBuckets`.

A service may import another service; `workspace/sidebarPreferenceService`
reads `platform/browserStorageService`, and `usage/usageChartService` labels
its bars with `usage/usageFormatService`. Keep those acyclic — the layer has
no cycles today.

## Keep the surface to what is called

Two services in `workspace/` rather than one: `workspaceSidebarService`
answers what the sidebar shows, `sidebarPreferenceService` what the user
folded and how that is remembered. They change for different reasons — a
grouping rule versus a storage key — and only the second one touches
localStorage.

The same rule applies inside a service. `usageRangeService` published eight
methods when it only had three to offer; the other five were internals that
leaked out when it stopped being a module of exported functions. If a method
has no caller outside the class, it is `private`, and it sits below the
contract it serves. If it has no caller at all, it goes.

A test is not a caller. When one drives a method nothing in the app uses, the
method is not the thing to keep — rewrite the test against the contract.

Tests sit beside the service they cover, and every service that holds a rule
worth pinning has one.
