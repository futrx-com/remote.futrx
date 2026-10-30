# RBAC frontend — reconnaissance, gap analysis, and plan

**Status:** planning (read-only reconnaissance complete; no code changed).

**Goal:** Expose the backend RBAC system in the frontend: view and manage roles,
assignments, bindings, and permission definitions. Enforce no business logic on
the frontend — all authorization decisions remain on the backend.

---

## Fixed constraints

- Do not refactor, rename, or restructure existing frontend code.
- Do not introduce a new state library, router, styling system, or component
  library. Use what exists (Preact, Zustand, Tailwind).
- Do not change existing components to make room for the new UI; add alongside.
- Do not invent new API endpoints. If the backend has no route, mark it as a
  backend dependency.
- **Mimic existing patterns exactly** — same folder layout, same naming
  conventions, same state approach, same API-call wrapper, same component
  composition, same Tailwind class strings. The existing codebase wins over any
  external convention.

---

## Phase 1 — Recon summary

### Project structure (verified)

Top-level folders under `frontend/src/`:

| Folder | Role |
|--------|------|
| `api/` | One exported object per domain (`projectApi`, `securityApi`, `userApi`…) |
| `app/` | Root, containers (data + logic wiring), route shell, extensions |
| `config/` | Constants and the `API_ROUTES` table (`config/routes.ts`) |
| `models/` | Pure TypeScript interfaces — data shapes only |
| `services/` | Stateless class singletons with co-located `.test.ts` |
| `state/` | Zustand stores (`stores/`), Preact hooks (`hooks/`), contexts (`context/`) |
| `transport/` | HTTP / WebSocket / upload primitives |
| `types/` | Wire-format union types |
| `ui/` | All rendered components, grouped by feature |

### Routing

No URL router. Navigation is `WorkspaceUiState` — a reducer in
`state/context/workspaceUiState.ts` that drives `pushState`/`replaceState`.
Three views: `"chat"`, `"settings"`, `"project-containers"`.
Settings sub-pages are `SettingsTab` values (string union in `models/workspace.ts`).
A new settings tab requires:

1. Add a value to `SettingsTab` in `models/workspace.ts`.
2. Add a tab entry to the `tabs` array in `ui/settings/SettingsPage.tsx`.
3. Add a conditional `{activeTab === "…" && <Section />}` in `SettingsPage`.
4. Wire data into `app/containers/SettingsContainer.tsx`.

### API layer

Single wrapper: `api/apiRequest.ts` → `requestJson<T>(method, url, body?)`.

- 401 → hard reload (never thrown to callers).
- Any other non-2xx → `throw new ApiError(message, status)`.
- `ApiError.status` is readable; `isDefinitiveRejection` returns true for 4xx.
- **403 is not differentiated from other 4xx today.** The backend currently
  returns 500 on denial; once it maps `ErrDenied` → 403, the UI can detect it
  via `cause instanceof ApiError && cause.status === 403`.

All endpoint strings live in `config/routes.ts` under `API_ROUTES`. API modules
import them. Example (`api/project/projectAccessApi.ts`):

```ts
export const projectAccessApi = {
  listAccess: (id) => requestJson<string[]>("GET", API_ROUTES.projects.access(id)),
  addAccess:  (id, email) =>
    requestJson<{ email: string }>("POST", API_ROUTES.projects.access(id), { email }),
  removeAccess: (id, email) =>
    requestJson<{ ok: boolean }>("DELETE", API_ROUTES.projects.accessMember(id, email)),
};
```

### State pattern

- Global state: vanilla Zustand store in `stores/<domain>/`.
- Per-screen state: `useState` inside a hook in `hooks/<domain>/`.
- No global cache (no React Query). Each hook fetches on mount / relevant prop
  change and owns `{ loading: boolean; data?: T; error?: string }`.
- Admin-gating: `{isAdmin && <AdminComponent />}` or ternary with
  `<SettingsNotice>` (plain notice at bottom of `SettingsPage.tsx`).

### Auth model (current)

`AuthSession.isAdmin: boolean` is the only permission signal in the frontend.
No RBAC abstraction, no `hasPermission`, no granular roles beyond admin/member.

```ts
// models/user.ts
export type UserRole = "admin" | "member";
export interface User { email: string; role: UserRole; addedAt: number; addedBy?: string; }
```

### Testing

```
"test": "node --experimental-strip-types --test"
```

Node built-in test runner. Tests live beside the module they cover (`*.test.ts`).
No harness for hooks or components — only pure-function modules are testable.
Pattern: `import assert from "node:assert/strict"; import test from "node:test";`

### Styling

Tailwind utility classes only. Design tokens: `text-ink-*`, `border-line`,
`bg-surface`, `bg-inset`, `text-accent-red`, `text-accent-blue`, etc.
No CSS modules, no styled-components.

---

## Phase 1b — UI design patterns (extracted verbatim from existing components)

Every new RBAC component must match these patterns character-for-character in
structure. The class strings below are taken directly from the working tree;
do not substitute synonyms or reorder Tailwind utilities.

### Section card (settings panel)

Source: `ui/account/UsersPanel.tsx`

```tsx
// Outer card — matches UsersPanel, SecuritySettings sub-sections
<section class="rounded-card border border-line bg-surface overflow-hidden">
  <header class="px-4 py-3 flex items-start gap-3 border-b border-line">
    <div class="flex-1 min-w-0">
      <div class="text-[14.5px] font-semibold text-ink-50">{title}</div>
      <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">{description}</div>
    </div>
    {loading && <Loader class="w-4 h-4 mt-2 text-ink-300 animate-spin" />}
  </header>
  <div class="p-3 space-y-3">
    {children}
  </div>
</section>
```

Non-admin notice (identical to `SettingsPage.tsx` `SettingsNotice`):

```tsx
<section class="rounded-card border border-line bg-surface p-4 text-[13px] leading-relaxed text-ink-300">
  Permissions are managed by server administrators.
</section>
```

### Row item

Source: `ui/account/UsersPanel.tsx` `UserRow` and
`ui/projects/project-containers/ProjectSharingSection.tsx` `MemberRow`

```tsx
// Standard list row — used for roles, assignments, bindings
<div class="rounded-md border border-line bg-tint px-3 py-2 space-y-1">
  <div class="flex items-center gap-2 min-w-0">
    <span class="text-[12.5px] text-ink-50 truncate" title={primaryText}>
      {primaryText}
    </span>
    {/* badge for role/scope/effect */}
    <span class={`inline-flex items-center h-5 px-1.5 rounded text-[11px] font-medium ${
      effect === "allow"
        ? "text-accent-green bg-accent-green/[0.10]"
        : "text-accent-red bg-accent-red/[0.10]"
    }`}>
      {effect}
    </span>
    <div class="ml-auto flex items-center gap-1">
      {/* action buttons — see Button patterns below */}
    </div>
  </div>
  {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
</div>
```

Badge colours (all from `UsersPanel.tsx` role badge):

| Semantic | Classes |
|----------|---------|
| allow / active / green | `text-accent-green bg-accent-green/[0.10]` |
| deny / danger / red | `text-accent-red bg-accent-red/[0.10]` |
| admin / highlight / blue | `text-accent-blue bg-accent-blue/[0.14]` |
| neutral / muted | `text-ink-300 bg-tint` |

### Empty and loading states

Source: `ui/projects/project-containers/ProjectContainerPrimitives.tsx`

```tsx
// Loading state
<div class="rounded-md border border-line bg-tint px-3 py-4 text-center text-[12.5px] text-ink-300">
  Loading roles…
</div>

// Empty state (compact variant)
<div class="rounded-card border border-line bg-surface px-3 py-2.5 text-sm text-ink-300">
  No roles defined yet.
</div>
```

For section-level loading (whole tab not yet ready), use the spinner pattern
from `SecuritySettings.tsx`:

```tsx
if (loading && !data) {
  return (
    <div class="flex items-center gap-2 text-[13px] text-ink-300">
      <Loader class="w-4 h-4 animate-spin" /> Loading permissions…
    </div>
  );
}
```

### Error banner

Source: `ui/account/UsersPanel.tsx` and `ui/projects/project-containers/ProjectSharingSection.tsx`

```tsx
// Used at section and row level — identical class string in both
{error && (
  <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
    <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
    <div class="text-accent-red break-words">{error}</div>
  </div>
)}
```

### Inline add form

Source: `ui/account/UsersPanel.tsx` `AddUserForm` and
`ui/projects/project-containers/ProjectSharingSection.tsx` `AddMemberForm`

```tsx
<form onSubmit={submit} class="rounded-md border border-line bg-tint p-2.5 space-y-2">
  <div class="grid gap-2 sm:grid-cols-[2fr_auto_auto] items-center">
    <input
      type="text"
      value={value}
      onInput={(e) => setValue((e.target as HTMLInputElement).value)}
      placeholder="…"
      spellcheck={false}
      autoComplete="off"
      class="h-9 px-2.5 rounded border border-line bg-inset text-[13px] text-ink-50 placeholder-ink-400 focus:outline-none focus:border-accent-blue/50"
    />
    {/* additional selects follow the same h-9 pattern */}
    <select
      value={selectValue}
      onChange={(e) => setSelectValue((e.target as HTMLSelectElement).value)}
      class="h-9 px-2 rounded border border-line bg-inset text-[13px] text-ink-50 focus:outline-none focus:border-accent-blue/50"
    >
      <option value="allow">allow</option>
      <option value="deny">deny</option>
    </select>
    <button
      type="submit"
      disabled={submitting}
      class="btn btn-primary btn-sm disabled:opacity-50"
    >
      {submitting ? "Adding…" : "Add"}
    </button>
  </div>
  {err && <div class="text-[11.5px] text-accent-red">{err}</div>}
</form>
```

### Row action buttons

Source: `ui/account/UsersPanel.tsx` `UserRow`

```tsx
{/* text action — e.g. "edit", "promote" */}
<button
  type="button"
  onClick={handleEdit}
  disabled={busy}
  class="h-7 px-2 rounded text-[11px] text-ink-300 hover:text-ink-100 hover:bg-tint-strong disabled:opacity-50"
>
  edit
</button>

{/* icon-only destructive action — e.g. remove */}
<button
  type="button"
  onClick={handleRemove}
  disabled={busy}
  class="h-7 w-7 rounded text-ink-300 hover:text-accent-red hover:bg-tint-strong grid place-items-center disabled:opacity-50"
  aria-label={`Remove ${name}`}
  title="Remove"
>
  <X class="w-3.5 h-3.5" />
</button>
```

### Modal chrome

Source: `ui/projects/CreateProjectModal.tsx` and `ui/projects/DeleteProjectModal.tsx`

All modals share the same shell. Do not deviate from these class strings.

```tsx
// Backdrop + centering wrapper
<div class="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-8">
  {/* click-away backdrop */}
  <div
    class="absolute inset-0 bg-black/55 backdrop-blur-[3px] modal-backdrop-fade"
    onClick={close}
  />

  {/* card */}
  <div
    role="dialog"
    aria-modal="true"
    aria-labelledby="modal-title-id"
    class="theme-menu-surface modal-card-pop relative w-full max-w-[480px] overflow-hidden rounded-[14px] border border-line bg-ink-800 text-ink-50 shadow-[0_24px_64px_rgba(0,0,0,.6)]"
  >
    {/* header */}
    <div class="flex items-start justify-between gap-4 px-5 pb-3.5 pt-[18px]">
      <div class="flex flex-col gap-[3px]">
        <div id="modal-title-id" class="text-[15px] font-semibold tracking-[-0.01em]">
          {title}
        </div>
        <div class="text-[12.5px] text-ink-300">{subtitle}</div>
      </div>
      <button
        type="button"
        onClick={close}
        disabled={busy}
        aria-label="Close"
        class="flex h-7 w-7 shrink-0 items-center justify-center rounded-[7px] text-ink-300 transition-colors hover:bg-tint hover:text-ink-100 disabled:opacity-45"
      >
        <X class="h-4 w-4" />
      </button>
    </div>

    {/* body — separated by top border */}
    <div class="flex flex-col gap-3.5 border-t border-line p-5">
      {children}
    </div>

    {/* footer — neutral background */}
    <div class="flex items-center justify-end gap-2 border-t border-line bg-tint px-5 py-3.5">
      {/* cancel */}
      <button
        type="button"
        onClick={close}
        disabled={busy}
        class="rounded-lg border border-line-strong px-3.5 py-2 text-[13px] text-ink-200 transition-colors hover:bg-tint hover:text-ink-100 disabled:opacity-45"
      >
        Cancel
      </button>
      {/* primary (neutral) */}
      <button
        type="button"
        onClick={() => void submit()}
        disabled={!canSubmit || busy}
        class="inline-flex items-center gap-[7px] rounded-lg border border-accent-blue/40 bg-accent-blue px-[15px] py-2 text-[13px] font-semibold text-on-accent transition-colors hover:bg-accent-blue/90 disabled:cursor-not-allowed disabled:border-line disabled:bg-tint-strong disabled:text-ink-400"
      >
        {busy && <Loader class="h-3.5 w-3.5 animate-spin" />}
        {busy ? "Saving…" : "Save"}
      </button>
    </div>
  </div>
</div>
```

Danger modal primary button (delete/revoke actions):

```tsx
{/* primary (danger) — from DeleteProjectModal.tsx */}
<button
  type="button"
  onClick={() => void submit()}
  disabled={!canSubmit || busy}
  class="inline-flex items-center gap-[7px] rounded-lg border border-accent-red/40 bg-accent-red px-[15px] py-2 text-[13px] font-semibold text-ink-900 transition-colors hover:bg-accent-red/90 disabled:cursor-not-allowed disabled:border-line disabled:bg-tint-strong disabled:text-ink-400"
>
  {busy && <Loader class="h-3.5 w-3.5 animate-spin" />}
  {busy ? "Removing…" : "Remove"}
</button>
```

### Form field inside a modal

Source: `ui/projects/CreateProjectModal.tsx`

```tsx
<div class="flex flex-col gap-[7px]">
  <label for="field-id" class="text-xs uppercase tracking-[0.08em] text-ink-300">
    Field label
  </label>
  <input
    id="field-id"
    value={value}
    onInput={(e) => setValue((e.target as HTMLInputElement).value)}
    placeholder="…"
    autocomplete="off"
    spellcheck={false}
    disabled={busy}
    class={`theme-submenu-surface w-full rounded-[9px] border bg-raised px-3 py-2.5 text-sm text-ink-100 outline-none transition-[border-color,box-shadow] duration-150 ${
      showError
        ? "border-accent-red/60 shadow-[0_0_0_3px_rgba(255,123,114,.12)]"
        : "border-line-strong"
    }`}
  />
  {showError && <div class="text-xs text-accent-red">{errorMessage}</div>}
</div>
```

Danger confirmation input (matches `DeleteProjectModal.tsx`):

```tsx
<input
  class="theme-submenu-surface w-full rounded-[9px] border border-line-strong bg-raised px-3 py-2.5 font-mono text-sm text-ink-100 outline-none transition-[border-color,box-shadow] duration-150 focus:border-accent-red/60 focus:shadow-[0_0_0_3px_rgba(255,123,114,.12)]"
/>
```

### Danger warning box (inside modal body)

Source: `ui/projects/DeleteProjectModal.tsx`

```tsx
<div class="rounded-[10px] border border-accent-red/25 bg-accent-red/[0.07] px-3.5 py-3 text-[13px] leading-5 text-ink-200">
  {warningText}
</div>
```

### `useDismissShortcut`

Every modal that handles Escape must use `useDismissShortcut` from
`state/hooks/shared/useDismissShortcut.ts`:

```ts
useDismissShortcut(close, { enabled: open });
```

The close function must check `busy` before acting:

```ts
function close() { if (!busy) onClose(); }
```

### Icon imports

All icons come from `ui/primitives/icons`. The icons used in analogous RBAC
components are:

| Usage | Icon |
|-------|------|
| Loading spinner | `Loader` |
| Close modal / remove row | `X` |
| Error banner | `AlertCircle` |
| Permissions tab nav | `ShieldCheck` (already imported in `SettingsPage`) |
| Role concept | `ShieldCheck` |
| User/assignment | `Users` (already imported in `SettingsPage`) |
| Checkmark badge (self row) | `Check` |

Import pattern: `import { AlertCircle, Check, Loader, X } from "../primitives/icons";`

### `useConfirm` for destructive actions

Removals that are not already in a dedicated delete modal use `useConfirm`:

```ts
// From ui/account/UsersPanel.tsx and ProjectSharingSection.tsx
const confirm = useConfirm();

const remove = async () => {
  await confirm({
    title: "Remove assignment",
    description: "The user loses this permission immediately.",
    message: `${email} — ${permission} at ${scope} will be removed.`,
    confirmLabel: "Remove",
    pendingLabel: "Removing…",
    action: () => onRemove(...),
  });
};
```

---

## Phase 2 — Gap analysis

### RBAC surfaces — what exists

| Surface | Frontend page/component/route | API call |
|---------|-------------------------------|----------|
| View roles | None | None |
| Create / edit / delete roles | None | None |
| View assignments | None | None |
| Create / revoke assignments | None | None |
| View role bindings | None | None |
| Create / remove bindings | None | None |
| View permission definitions | None | None |
| View audit log | None | None |
| Show why a decision was denied | None | None |

### Backend HTTP routes for RBAC — verified missing

Inspected `backend/internal/transport/http/server.go` (`Handlers` struct) and
the full handler directory (`backend/internal/transport/http/handlers/`).

**No handler file exists for RBAC.** The RBAC service is only wired into
middleware (actor propagation) and the project service. Not exposed over HTTP.

Confirmed registered permissions (from `registry.go` and `project/permissions.go`):

| Key | Description | Scope | Baseline |
|-----|-------------|-------|----------|
| `permissions.assignments.manage` | Manage assignments and bindings | platform | none |
| `permissions.roles.manage` | Manage custom roles | platform | none |
| `projects.lifecycle.manage` | Start/stop/restart/repair container | project | project-member |
| `projects.access.manage` | List/add/remove project members | project | project-member |

### 403 differentiation

Frontend does not differentiate 403 from any other 4xx. Backend maps `ErrDenied`
to an unknown HTTP status (no transport mapping found in handler files). Until
the backend maps `ErrDenied` → HTTP 403, the "why denied" UI cannot be built.

---

## Phase 3 — Plan

### Step 0 — Backend prerequisite (all frontend work blocked)

| Method | Path | Maps to |
|--------|------|---------|
| `GET` | `/api/admin/permissions/definitions` | `registry.Definitions()` |
| `GET` | `/api/admin/permissions/state` | full `State{Roles, Assignments, Bindings}` |
| `POST` | `/api/admin/permissions/roles` | `Service.CreateRole` |
| `PATCH` | `/api/admin/permissions/roles/:id` | `Service.UpdateRole` |
| `DELETE` | `/api/admin/permissions/roles/:id` | `Service.DeleteRole` |
| `POST` | `/api/admin/permissions/assignments` | `Service.SetAssignment` |
| `DELETE` | `/api/admin/permissions/assignments` | `Service.RemoveAssignment` (by body) |
| `POST` | `/api/admin/permissions/bindings` | `Service.BindRole` |
| `DELETE` | `/api/admin/permissions/bindings` | `Service.UnbindRole` (by body) |

Also required before Step 5: backend maps `ErrDenied` → HTTP 403.

### Step 1 — Models, routes, API module

**`frontend/src/models/rbac.ts`** (new file):

```ts
export type RbacEffect = "allow" | "deny";
export type RbacScopeKind = "platform" | "project";
export interface RbacScope { kind: RbacScopeKind; id?: string; }
export interface RbacRoleRule { permission: string; effect: RbacEffect; }
export interface RbacDefinition {
  key: string; description: string;
  scopes: RbacScopeKind[]; baseline: string; delegable: boolean;
}
export interface RbacRole {
  id: string; name: string; description: string;
  rules: RbacRoleRule[]; createdBy: string; createdAt: number; updatedAt: number;
}
export interface RbacAssignment {
  id: string; userEmail: string; permission: string;
  effect: RbacEffect; scope: RbacScope; createdBy: string; createdAt: number;
}
export interface RbacBinding {
  id: string; roleId: string; userEmail: string;
  scope: RbacScope; createdBy: string; createdAt: number;
}
export interface RbacState {
  roles: RbacRole[];
  assignments: RbacAssignment[];
  bindings: RbacBinding[];
}
```

**`frontend/src/config/routes.ts`** additions inside `API_ROUTES`:

```ts
permissions: {
  definitions: "/api/admin/permissions/definitions",
  state:       "/api/admin/permissions/state",
  roles:       "/api/admin/permissions/roles",
  role:        (id: string) => `/api/admin/permissions/roles/${encodeURIComponent(id)}`,
  assignments: "/api/admin/permissions/assignments",
  bindings:    "/api/admin/permissions/bindings",
},
```

**`frontend/src/api/permissionsApi.ts`** (new file, mirrors `securityApi.ts`):

```ts
import { requestJson } from "./apiRequest";
import type { RbacDefinition, RbacState, RbacRole, RbacAssignment, RbacBinding } from "../models/rbac";
import { API_ROUTES } from "../config/routes";

export const permissionsApi = {
  listDefinitions: () =>
    requestJson<RbacDefinition[]>("GET", API_ROUTES.permissions.definitions),
  fetchState: () =>
    requestJson<RbacState>("GET", API_ROUTES.permissions.state),
  createRole: (body: { name: string; description: string; rules: { permission: string; effect: string }[] }) =>
    requestJson<RbacRole>("POST", API_ROUTES.permissions.roles, body),
  updateRole: (id: string, body: { name: string; description: string; rules: { permission: string; effect: string }[] }) =>
    requestJson<RbacRole>("PATCH", API_ROUTES.permissions.role(id), body),
  deleteRole: (id: string) =>
    requestJson<{ ok: boolean }>("DELETE", API_ROUTES.permissions.role(id)),
  setAssignment: (body: { userEmail: string; permission: string; effect: string; scope: { kind: string; id?: string } }) =>
    requestJson<RbacAssignment>("POST", API_ROUTES.permissions.assignments, body),
  removeAssignment: (body: { userEmail: string; permission: string; scope: { kind: string; id?: string } }) =>
    requestJson<void>("DELETE", API_ROUTES.permissions.assignments, body),
  bindRole: (body: { roleId: string; userEmail: string; scope: { kind: string; id?: string } }) =>
    requestJson<RbacBinding>("POST", API_ROUTES.permissions.bindings, body),
  unbindRole: (body: { roleId: string; userEmail: string; scope: { kind: string; id?: string } }) =>
    requestJson<void>("DELETE", API_ROUTES.permissions.bindings, body),
};
```

### Step 2 — Read-only permissions tab

**`models/workspace.ts`** — extend `SettingsTab`:

```ts
export type SettingsTab =
  | "appearance" | "notifications" | "agents" | "usage"
  | "users" | "security" | "applications" | "updates" | "info"
  | "permissions"; // new
```

**`ui/settings/SettingsPage.tsx`** — add tab entry (additive, no edits to
existing entries):

```ts
{ id: "permissions", label: "Permissions", description: "Roles, assignments, and bindings.", Icon: ShieldCheck },
```

Add panel conditional (additive):

```tsx
{activeTab === "permissions" && (
  isAdmin ? (
    <PermissionsSettings ... />
  ) : (
    <SettingsNotice>
      Permissions are managed by server administrators.
    </SettingsNotice>
  )
)}
```

**`state/hooks/permissions/usePermissions.ts`** (new file, mirrors
`useSecuritySettings.ts` structure):

```ts
export interface PermissionsState {
  loading: boolean;
  definitions: RbacDefinition[];
  roles: RbacRole[];
  assignments: RbacAssignment[];
  bindings: RbacBinding[];
  error: string | null;
}

export function usePermissions(enabled: boolean): PermissionsState & { refresh: () => Promise<void> } {
  const [state, setState] = useState<PermissionsState>({
    loading: false, definitions: [], roles: [], assignments: [], bindings: [], error: null,
  });

  const load = useCallback(async () => {
    setState((s) => ({ ...s, loading: true, error: null }));
    try {
      const [definitions, { roles, assignments, bindings }] = await Promise.all([
        permissionsApi.listDefinitions(),
        permissionsApi.fetchState(),
      ]);
      setState({ loading: false, definitions, roles, assignments, bindings, error: null });
    } catch (err) {
      setState((s) => ({ ...s, loading: false, error: (err as Error).message }));
    }
  }, []);

  useEffect(() => { if (enabled) void load(); }, [enabled]);
  return { ...state, refresh: load };
}
```

**`ui/settings/PermissionsSettings.tsx`** (new file):

Top-level pattern follows `SecuritySettings.tsx`:
1. Loading spinner when `loading && !definitions.length`.
2. Error banner when `error`.
3. Four sub-sections in `<div class="space-y-4">`.

```tsx
export function PermissionsSettings({ permissions }: { permissions: ReturnType<typeof usePermissions> }) {
  const { loading, error, definitions, roles, assignments, bindings } = permissions;

  if (loading && !definitions.length) {
    return (
      <div class="flex items-center gap-2 text-[13px] text-ink-300">
        <Loader class="w-4 h-4 animate-spin" /> Loading permissions…
      </div>
    );
  }

  return (
    <div class="space-y-4">
      {error && (
        <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
          <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
          <div class="text-accent-red break-words">{error}</div>
        </div>
      )}
      <RolesList roles={roles} loading={loading} />
      <AssignmentsList assignments={assignments} loading={loading} />
      <BindingsList bindings={bindings} roles={roles} loading={loading} />
      <DefinitionsList definitions={definitions} />
    </div>
  );
}
```

**New sub-components** under `ui/settings/permissions/`:

Each uses the section card pattern (`rounded-card border border-line bg-surface
overflow-hidden`) with header + body.

`RolesList.tsx` — rows show: role name (primary text) + rule count badge.
Loading: `<Loading text="Loading roles…" />` (same primitive).
Empty: `<Empty text="No roles defined yet." compact />`.

`AssignmentsList.tsx` — rows show: user email | permission key | effect badge
(green=allow, red=deny) | scope badge.

`BindingsList.tsx` — rows show: user email | role name | scope badge.

`DefinitionsList.tsx` — read-only; rows show: permission key (mono) |
description | scope | baseline | delegable indicator.

### Step 3 — Role mutations

**`ui/settings/permissions/createRoleForm.ts`** (new, co-located validation,
mirrors `ui/projects/createProjectForm.ts`):

```ts
class CreateRoleFormLogic {
  validate(name: string, description: string, rules: RbacRoleRule[], existing: RbacRole[]): RoleFormValidation {
    // name: 1–80 chars, non-empty, unique (case-insensitive)
    // rules: at least 1, no duplicate permission keys
    // returns { ok, errors: Record<field, string> }
  }
}
export const createRoleForm = new CreateRoleFormLogic();
```

**`ui/settings/permissions/createRoleForm.test.ts`** — covers validation rules.

**`ui/settings/permissions/CreateRoleModal.tsx`** — uses modal chrome from
Phase 1b. Body:
- Name field (text input, modal field pattern)
- Description field (textarea, same border/shadow classes as input)
- Rules list: each row is a grid with permission `<select>` + effect radio
  (`allow`/`deny`) + remove icon button. "Add rule" is a text action button:
  `class="h-7 px-2 rounded text-[11px] text-ink-300 hover:text-ink-100 hover:bg-tint-strong"`

**`ui/settings/permissions/EditRoleModal.tsx`** — same form, pre-populated.

**`ui/settings/permissions/DeleteRoleModal.tsx`** — mirrors `DeleteProjectModal.tsx`
exactly:
- Danger warning box with role name in `<span class="font-mono text-ink-50">`.
- Typed confirmation input (mono, red focus ring).
- Danger footer button (`bg-accent-red`).
- Optional "also remove all bindings" checkbox above the input when the role has bindings.

### Step 4 — Assignment and binding mutations

**`ui/settings/permissions/CreateAssignmentModal.tsx`** — uses modal chrome.
Fields:
- User email (text, modal field pattern, validated against registered users)
- Permission (select from `definitions`, mono option text)
- Effect (radio: `allow` / `deny`, rendered as two small buttons toggling
  `bg-accent-green/[0.10] text-accent-green` / `bg-accent-red/[0.10] text-accent-red`)
- Scope kind (radio: `platform` / `project`)
- Project id (select, visible only when scope = `project`, populated from
  `WorkspaceContext.projects`)

Removal of an assignment from the list uses `useConfirm` (no separate modal).

**`ui/settings/permissions/CreateBindingModal.tsx`** — uses modal chrome.
Fields: user email, role `<select>`, scope kind + project select (same pattern
as `CreateAssignmentModal`).

Removal uses `useConfirm`.

### Step 5 — "Why denied" UI

Do not implement until backend maps `ErrDenied` → HTTP 403.

Once it does, add to `api/apiError.ts`:

```ts
export function isPermissionDenied(cause: unknown): boolean {
  return cause instanceof ApiError && cause.status === 403;
}
```

In mutation error displays, branch on `isPermissionDenied(submitError)` before
the generic error message. No other change.

### Ordered implementation table

| Step | Deliverable | Blocked on |
|------|-------------|-----------|
| 0 | Backend HTTP routes (9 endpoints) | backend work |
| 0b | Backend `ErrDenied` → HTTP 403 | backend work |
| 1 | `models/rbac.ts`, routes additions, `api/permissionsApi.ts` | Step 0 |
| 2 | `SettingsTab` addition, `usePermissions` hook, `PermissionsSettings.tsx` + 4 read-only sub-components | Step 1 |
| 3 | `createRoleForm.ts` + test, `CreateRoleModal`, `EditRoleModal`, `DeleteRoleModal` | Step 2 |
| 4 | `CreateAssignmentModal`, `CreateBindingModal`, `usePermissions` mutation callbacks | Step 3 |
| 5 | `isPermissionDenied`, "you don't have permission" branch | Step 0b |

Each step is independently reviewable. Steps 3 and 4 can be combined.

---

## Phase 4 — Risks and open questions

### Pattern conflicts

1. **No reusable repeating-field component.** Role rules need a dynamic list of
   rows (permission + effect). A `RuleRow` sub-component lives inside
   `CreateRoleModal.tsx` — domain-specific, not promoted to `ui/primitives/`.

2. **Permission key `<select>` requires definitions loaded before the modal
   opens.** `usePermissions` loads when the tab is active, before any modal
   opens — so this is satisfied by the tab-activation lifecycle.

3. **Scope field for assignments / bindings.** Project scope requires a project
   ID. The project list is available from `WorkspaceContext`; the modal reads it
   from there, consistent with how `ProjectSharingSection` receives its project.

4. **No test harness for hooks or components** (`state/README.md` is explicit).
   Tests are written only for `createRoleForm.ts` (pure validation) and any
   additional pure helpers. `usePermissions` is not testable under the current
   setup.

### Backend dependencies

| Backend change | Blocks |
|---------------|--------|
| `GET /api/admin/permissions/definitions` | All frontend RBAC |
| `GET /api/admin/permissions/state` | All frontend RBAC |
| `POST/PATCH/DELETE /api/admin/permissions/roles` | Steps 3+ |
| `POST/DELETE /api/admin/permissions/assignments` | Step 4 |
| `POST/DELETE /api/admin/permissions/bindings` | Step 4 |
| `ErrDenied` → HTTP 403 | Step 5 |
| Audit log HTTP endpoint (none observed) | Audit log UI (not planned) |

### Assumptions to confirm before coding

1. The permissions tab is **admin-only** in the UI. Non-admin users who hold
   `permissions.*.manage` via an explicit RBAC assignment remain gated out.
   Confirm, or specify whether the tab should be visible to any user holding the
   management permission.

2. The project ID used in `scope.id` for project-scoped assignments/bindings is
   the same `project.id` the workspace WebSocket uses (not the slug).

3. `GET /api/admin/permissions/state` returns roles, assignments, and bindings
   in one response. If the backend prefers three separate endpoints, `fetchState`
   becomes three parallel `Promise.all` calls — the hook structure does not
   change.

4. `deleteRole` with existing bindings returns an error when `unbind` is not
   set. The `DeleteRoleModal` should offer a "also remove all bindings" checkbox.
   Confirm this is the intended UX.

### Open questions

1. What HTTP status does `rbac.ErrDenied` produce at the transport layer today?
   No transport mapping was found. If it is 500, Step 5 cannot land until it
   changes.

2. Should the permissions tab ever be visible to non-admin users (e.g., showing
   only their own effective permissions)? The current plan is admin-only.

3. Is there an HTTP endpoint planned for `permission-audit.jsonl`? None found.
   Audit log UI is deferred.

4. Should the project sharing tab (`ProjectSharingSection`) eventually show
   which RBAC roles a member holds in addition to plain membership? This
   affects Step 4 scope.

---

## First PR

**Contains:** Steps 1 + 2 — `models/rbac.ts`, `config/routes.ts` additions,
`api/permissionsApi.ts`, the `"permissions"` `SettingsTab` value,
`usePermissions` hook (read-only), and the five components:
`PermissionsSettings.tsx`, `RolesList.tsx`, `AssignmentsList.tsx`,
`BindingsList.tsx`, `DefinitionsList.tsx`.

All components strictly follow the UI patterns documented in Phase 1b.

**Blocked on:** the backend exposing at minimum
`GET /api/admin/permissions/definitions` and
`GET /api/admin/permissions/state`. Without those two routes no component
can render anything meaningful.
