Contract for the permission policy HTTP surface, implemented by the backend in
transport/http/handlers/permissions_handler.go. If a path or shape changes, only
this file, config/routes.ts and permissions.ts change; the UI does not.

Status legend: **verified** = visible in `backend/internal/rbac`.

## Types (field names from `backend/internal/rbac/models`)

All records carry json tags in Go (`models/state.go`, `models/definition.go`), so the
casing below is the wire casing: lowerCamelCase of the Go field names
(`ID` -> `id`, `UserEmail` -> `userEmail`, `RoleID` -> `roleId`).

| Type        | Fields                                                                                  |
|-------------|-----------------------------------------------------------------------------------------|
| Scope       | `kind` ("platform" \| "project"), `id?` (platform takes no id; project id `^[a-z0-9][a-z0-9_-]{0,63}$`) |
| Definition  | `key`, `description`, `scopes: ScopeKind[]`, `baseline` ("none" \| "admin" \| "project-member" \| "authenticated"), `delegable` |
| RoleRule    | `permission`, `effect` ("allow" \| "deny")                                              |
| Role        | `id`, `name`, `description`, `rules: RoleRule[]`, `createdBy`, `createdAt`, `updatedAt` |
| Assignment  | `id`, `userEmail`, `permission`, `effect`, `scope`, `createdBy`, `createdAt`            |
| RoleBinding | `id`, `roleId`, `userEmail`, `scope`, `createdBy`, `createdAt`                          |

Timestamps are Unix milliseconds (`UnixMilli()` in the service) — verified.
Ids are server-assigned (`newID()`) — verified.

## Reads

```
GET /api/admin/permissions/definitions  -> 200 { definitions: Definition[] }, 403 denied
GET /api/admin/permissions/roles        -> 200 { roles: Role[] }
GET /api/admin/permissions/assignments  -> 200 { assignments: Assignment[] }
GET /api/admin/permissions/bindings     -> 200 { bindings: RoleBinding[] }
```

Denials on these routes are 403. The UI treats any non-2xx as an error and does
NOT special-case 403 yet.

Definitions are code-owned (registry). GET only; there are no write routes and
there never will be.

## Writes

Create and duplicate both return **200**: the service does not report whether a
record already existed.

```
POST /api/admin/permissions/assignments
  body { userEmail, permission, scope: { kind, id }, effect }
  Upsert: same effect is a no-op, different effect updates. Returns the assignment. 200.

DELETE /api/admin/permissions/assignments
  ?userEmail=<email>&permission=<key>&scopeKind=<kind>&scopeId=<id>
  204, idempotent. scopeId is omitted for platform scope. There is no delete-by-id.

POST /api/admin/permissions/bindings
  body { userEmail, roleId, scope: { kind, id } }
  Idempotent. Returns the binding. 200.

DELETE /api/admin/permissions/bindings
  ?userEmail=<email>&roleId=<id>&scopeKind=<kind>&scopeId=<id>
  204, idempotent. scopeId is omitted for platform scope. There is no delete-by-id.

POST /api/admin/permissions/roles
  body { name, description, rules: [{ permission, effect }] }
  Server-assigned id. Returns the role. 200.

PUT /api/admin/permissions/roles/{id}
  body { name, description, rules }. Returns the role.

DELETE /api/admin/permissions/roles/{id}?unbind=true
  204 / 409 if bound and unbind not set. Deleting a missing role is 204.
```

## Error mapping (new routes only)

| Service error          | Status |
|------------------------|--------|
| ErrActorRequired       | 401    |
| ErrDenied              | 403    |
| ErrRoleNotFound        | 404    |
| ErrRoleInUse           | 409    |
| ErrInvalidRole, ErrInvalidScope, ErrInvalidEffect, ErrUnknownPermission, ErrUserNotRegistered | 400 |
| malformed body or query | 400   |
| anything else          | 500    |

Gating happens inside `rbac.Service`: reads use `requireRead` (either
management permission), assignment and binding writes require
`permissions.assignments.manage`, role writes require `permissions.roles.manage`.
`GET /definitions` is gated by `Service.Definitions` with the same `requireRead`.

## Status

Implemented by the backend in `transport/http/handlers/permissions_handler.go`.
Behaviour verified in `backend/internal/rbac`:
- `SetAssignment`: same effect is a no-op, a different effect updates the record.
- `BindRole`: binding the same (user, role, scope) twice is a no-op.
- `RemoveAssignment` / `UnbindRole` / `DeleteRole` of a missing record is a no-op.
- `DeleteRole` on a bound role fails with `ErrRoleInUse` unless `Unbind` is set.
- Role validation: name 1-80 chars and unique case-insensitively, description <= 500, at least one rule, no duplicate permission, known permission, valid effect.
- `BindRole` rejects roles whose rules do not support the scope kind.

Corrections from the first proposal: DELETE by `{id}` became natural-key query
parameters (the service removes by natural key); create and duplicate both return
200 instead of 201/200.
