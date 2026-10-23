# Service-owned permission abstraction

**Status:** done (implemented on `feat/permission-abstraction`, draft PR futrx-com/remote.futrx#223). QA deploy step blocked: see Outcome.

**Goal:** Add a reusable permission layer that lets Remote decide whether an existing authenticated user may perform a registered service action, supports explicit allow and deny assignments, groups registered permissions into custom roles, and lets an authorized user delegate permissions to another existing user. Authorization decisions must be made by services, not HTTP handlers.

**Initial delivery:** Backend permission domain, file persistence, composition, actor propagation, and service-side enforcement of two project permissions. This plan does not create a new user type, administrator type, authentication flow, or replacement for project membership.

---

## Fixed constraints

- Keep the current user directory and its `admin` / `member` roles. Do not introduce a second identity system.
- Treat the current administrator decision (`auth.Service.IsAdmin`) as the bootstrap/root policy.
- Define permission keys only in Go code. Runtime users may assign registered keys or collect them into custom roles, but may not invent keys.
- Put every authorization decision in the service that owns the protected operation.
- Do not add authorization branches to handlers and do not modify existing handler files in this delivery.
- Preserve existing project membership as resource visibility. A permission does not silently make a non-member able to see a project.
- Preserve existing behavior until an explicit assignment changes it.
- Default to deny for a newly registered permission that has no compatibility policy.
- Persist changes atomically and audit every successful permission/role mutation.

## Non-goals

- Creating, deleting, authenticating, or otherwise managing users.
- Replacing the existing `admin` and `member` user roles.
- Moving all existing authorization checks in one change.
- Field-level data masking or arbitrary expression-based ABAC.
- Letting an administrator define new permission names through the UI or API.
- Changing HTTP response mapping in the first delivery.
- Treating project membership as a custom role.

---

## Current architecture and gaps

The repository already has the identities and several useful authorization concepts, but no common permission vocabulary:

- `service/user` owns registered users and the fixed `admin` / `member` roles.
- `service/auth.Service.IsAdmin` recognizes both the local administrator and directory administrators.
- `service/project` owns a per-project membership list.
- `service/chat.AccessService`, `service/schedule`, `service/usage`, `service/skills`, and agent capability code each implement their own admin/member/owner decisions.
- `transport/http/handlers` contains hard-coded administrator checks for users, global applications, usage price management/rebuild, self-update, Google OAuth configuration, agent authentication, project deletion, and project limits.
- Project handlers perform a broad membership check before calling project service methods. The project service methods themselves generally do not authorize the caller.
- The auth middleware verifies registration but does not attach an application actor to the request context.

This means a direct call to many service methods can bypass the policy expressed in a handler. It also means a delegated permission cannot currently replace an `isAdmin` branch consistently.

---

## Permission registry pattern decision

**Decision: adopt the permission registry pattern, with a narrower responsibility than the whole permission system.**

The registry is a good fit for the rule that only Remote developers may add permissions. It gives developers one typed, validated catalog and makes duplicate keys, missing descriptions, unsupported scopes, and accidental renames fail during tests or startup.

The registry must not store assignments and must not decide access by itself. A complete design is:

```text
code-owned Registry
       |
       v
persisted Assignments + Custom Roles ---> Evaluator ---> owning Service
                                                ^
                                                |
                                      Actor + resource facts
```

Using a registry alone would not solve deny precedence, role membership, delegation, persistence, audit, or service enforcement. The suggestion is therefore accepted for cataloging, not as a substitute for an evaluator.

---

## Target package

Add `backend/internal/service/permission` as one bounded service package:

```text
backend/internal/service/permission/
  actor.go          authenticated/system actor context
  definition.go     keys, scopes, definitions, compatibility policies
  registry.go       immutable code-owned catalog
  decision.go       request, decision, reasons, stable errors
  evaluator.go      deny/allow/baseline evaluation
  roles.go          custom-role commands and validation
  assignments.go    direct grants and role bindings
  delegation.go     mutation authorization and anti-escalation rules
  ports.go          repository, identity, membership, and audit ports
```

Add one adapter package:

```text
backend/internal/stores/filepermissions/
  store.go
  records.go
  store_test.go
```

Do not put the permission domain under `service/user`: users are subjects consumed by the permission service, not owned by it.

### Core types

Use distinct types rather than free-form strings at call sites:

```go
type Key string
type Effect string
type ScopeKind string

const (
    Allow Effect = "allow"
    Deny  Effect = "deny"
)

type Scope struct {
    Kind ScopeKind
    ID   string
}

type Definition struct {
    Key         Key
    Description string
    Scopes      []ScopeKind
    Baseline    BaselinePolicy
    Delegable   bool
}

type Check struct {
    Permission Key
    Scope      Scope
}
```

Initial scope kinds are only `platform` and `project`. Do not add a generic attribute map in version one; it produces unvalidated policy strings and makes scope comparison unsafe. Add a new typed scope kind when a real service needs one.

### Naming convention

Use `<bounded-context>.<resource>.<action>`, for example:

- `projects.lifecycle.manage`
- `projects.access.manage`
- `permissions.assignments.manage`
- `permissions.roles.manage`

Keys are stable persisted identifiers. Renaming one requires an explicit store migration or a temporary alias declared in code.

### Developer registration API

Each owning service exports its definitions from a small `permissions.go` file. The composition root builds one immutable registry:

```go
registry := permission.MustRegistry(
    serviceproject.PermissionDefinitions()...,
    permission.ManagementDefinitions()...,
)
```

`MustRegistry` rejects empty/malformed keys, duplicate keys, empty descriptions, empty scope lists, unknown baseline policies, and a delegable definition that has no safe scope comparison. Tests should use the error-returning constructor.

There is deliberately no `CreatePermission` service method, repository method, DTO, or persisted permission-definition file.

---

## Subjects, assignments, and custom roles

The permission layer references existing users by normalized email. It does not own a `User` model.

Persist three kinds of records:

```go
type Assignment struct {
    ID         string
    UserEmail  string
    Permission Key
    Effect     Effect
    Scope      Scope
    CreatedBy  string
    CreatedAt  int64
}

type Role struct {
    ID          string
    Name        string
    Description string
    Rules       []RoleRule
    CreatedBy   string
    CreatedAt   int64
    UpdatedAt   int64
}

type RoleBinding struct {
    ID        string
    RoleID    string
    UserEmail string
    Scope     Scope
    CreatedBy string
    CreatedAt int64
}
```

A `RoleRule` contains a registered permission key and an effect. The role binding supplies the concrete scope, and every rule must support that scope kind. This keeps a role reusable while preventing a project role from accidentally becoming platform-wide.

Custom roles are permission bundles only. They must not be written into `user.User.Role`, and they must not be accepted by `user.NormalizeRole`.

Repository commands must be idempotent where practical:

- Upserting the same direct `(user, permission, scope)` replaces its effect.
- Binding the same `(user, role, scope)` twice is a no-op.
- Deleting a missing assignment/binding is a no-op.
- A role cannot be deleted while bindings exist unless the command explicitly requests an atomic delete-with-unbind.

The store validates referenced permission keys against the registry at the service boundary, not inside the file adapter.

---

## Evaluation semantics

For `Can(ctx, Check)`, evaluate in this order:

1. Require a valid actor. Human actors must reference an existing registered user.
2. A trusted internal `SystemActor` may call explicitly marked internal entry points; it is never created from HTTP input.
3. Existing administrators are allowed every registered permission. Mutable assignments cannot remove this root/break-glass capability.
4. Collect matching direct assignments and rules from matching role bindings.
5. If any matching rule is `deny`, deny.
6. Otherwise, if any matching rule is `allow`, allow.
7. Otherwise evaluate the definition's code-owned compatibility baseline.
8. Otherwise deny.

Scope matching is exact in version one:

- `platform` matches only `platform`.
- `project:<id>` matches only that project.

Do not make platform assignments implicitly match every project in the first version. If such inheritance is needed later, add it as an explicit, tested rule rather than an accident of string matching.

### Compatibility baselines

Baselines preserve current behavior during rollout:

- `BaselineAdmin`: current administrators only.
- `BaselineProjectMember`: administrators or membership in the checked project.
- `BaselineAuthenticated`: any registered actor.
- `BaselineNone`: no implicit access.

The registry owns which baseline applies to a permission because it is part of the code-defined behavior. The evaluator consumes narrow `IdentityDirectory` and `ProjectMembership` ports rather than importing concrete auth or project services.

Explicit deny overrides a member/authenticated baseline for non-admins. Explicit allow may add a capability, but it does not bypass independent resource visibility such as project membership.

Return a stable `permission.ErrDenied` to services. The evaluator may also return a structured `Decision` for logs/tests, but public errors must not reveal which role or deny record caused the result.

---

## Delegating permission management

Register two management permissions in code:

- `permissions.assignments.manage`: create/update/delete direct assignments and role bindings.
- `permissions.roles.manage`: create/update/delete custom roles.

An administrator has both through the root policy. A non-admin may receive either like any other registered permission.

Prevent privilege escalation with these rules inside the permission service:

1. Managing a direct assignment or role binding requires `permissions.assignments.manage` at platform scope.
2. Creating or changing a custom role requires `permissions.roles.manage` at platform scope.
3. A non-admin may assign only a permission that is marked `Delegable` and that the actor currently holds at the same scope.
4. A non-admin may bind a role only if the actor could delegate every rule in that role at the binding scope.
5. A non-admin may not create a role containing either management permission unless they already hold that exact management permission.
6. The service re-evaluates these rules within the same process lock/transaction used for the mutation so a concurrent change cannot create a time-of-check/time-of-use escalation.
7. The target email must already exist in the current user directory.

These rules mean “can give permissions” is itself a permission, while possession of that management permission is not an unrestricted route to permissions the delegator does not have.

Every mutation appends an audit event containing actor, operation, target, permission/role, scope, old effect, new effect, and timestamp. Never log session cookies or unrelated user data.

---

## Supplying the actor without handler authorization

Existing service signatures often lack a caller. Because handler files are frozen, propagate the actor through `context.Context`:

1. Add `permission.ContextWithActor` and `permission.ActorFromContext`.
2. After the existing auth middleware validates a session and registration, attach `Actor{Email: normalizedEmail}` to a derived request context before calling the next handler.
3. WebSocket upgrades inherit that request context.
4. Never trust an email already present in a request body, query, or service method parameter as the actor.
5. Tests call services with an explicit actor context.
6. Background jobs and composition adapters use an explicit `SystemActor` only at reviewed internal entry points.

This changes middleware plumbing but not handler files. The authorization decision still lives in the service.

Do not silently treat a missing actor as system. Protected public service methods fail closed with `ErrActorRequired`. Where existing internal code needs the same mechanics, split the method into an authorized public entry point and an unexported/internal operation, or pass a deliberate system context from the composition adapter.

---

## First two protected permissions

Start in `service/project`, where handlers already admit project members and the service owns the operations.

### 1. `projects.lifecycle.manage`

- Scope: `project`.
- Baseline: `BaselineProjectMember` to preserve current behavior.
- Delegable: yes.
- Enforce at the start of `Start`, `Stop`, `Restart`, and `RepairNetwork`.
- Use one private helper so every lifecycle operation builds the same check.

### 2. `projects.access.manage`

- Scope: `project`.
- Baseline: `BaselineProjectMember` to preserve current behavior.
- Delegable: yes.
- Enforce at the start of `ListAccess`, `AddAccess`, and `RemoveAccess`.
- Keep the current registered-user validation and last-member guardrail as separate domain invariants. Permission to manage access does not waive them.

Do not protect low-level `Get` in the first slice. It is heavily used as an internal lookup by other services; changing it before separating internal lookup from caller-visible read behavior risks breaking startup, scheduling, agent capability resolution, and background reconciliation.

The project service receives a narrow `Authorizer` interface:

```go
type Authorizer interface {
    Require(context.Context, permission.Check) error
}
```

The package may depend on permission models, but not on a concrete permission service or file store.

---

## Persistence and lifecycle cleanup

Use `DATA_DIR/permissions.json`, directory mode `0700`, file mode `0600`, an in-process mutex, temporary-file write, `fsync`, close, and atomic rename. Store a schema version from the start.

Use a separate append-only `DATA_DIR/permission-audit.jsonl` with mode `0600`. If the audit append fails, fail the permission mutation; an unaudited authorization change is worse than a rejected change.

On startup:

- Parse and validate the entire file.
- Reject duplicate record IDs and malformed scopes/effects.
- Reject references to unknown permission keys and unknown role IDs with an actionable startup error. Do not silently discard policy.
- Normalize stored emails.

Extend the existing user-removal cleanup composition so deleting a user removes their direct assignments and role bindings before deleting the user. Keep role definitions. Cleanup must be idempotent.

No data migration is needed for the initial release: absence of `permissions.json` means no explicit assignments or custom roles, and compatibility baselines preserve existing behavior.

---

## Composition changes

1. Add a permission repository/audit capability to `stores.Stores` and initialize `filepermissions` in `stores.New`.
2. Add it to `service.Dependencies`.
3. Build the registry before dependent services.
4. Construct the evaluator from the registry, permission repository, existing user/auth identity adapter, and a narrow project-membership adapter.
5. Inject the evaluator's narrow authorizer into the project service with an option or constructor dependency.
6. Expose the permission application service as `Services.Permissions` for future transports and operator tooling.
7. Add permission cleanup to `userRemovalCleanup`.
8. Attach authenticated actors in auth middleware after current session and registration validation.

Watch for the current construction cycle: project service is built before auth service, while auth wraps the user service and permission baselines need both admin status and project membership. Avoid late mutable wiring. Use narrow adapters over `user.Service` plus the local-admin identity source, or construct a small identity policy before the project service. The final graph must be acyclic and constructor-complete.

---

## Implementation sequence

### Phase 0 — characterization

- Add service tests proving current project members can perform lifecycle and access-list operations and non-members cannot reach them through the current HTTP routes.
- Add tests for local admin and directory admin recognition.
- Record all trusted background/internal callers of the project lifecycle methods.

### Phase 1 — domain and registry

- Add typed models, validation, registry, baseline enum, and registry tests.
- Register the two management permissions and two project permissions.
- Add compile/startup tests for duplicate and malformed definitions.

### Phase 2 — file store and audit

- Add versioned records, atomic persistence, corruption tests, permissions tests, and concurrent mutation tests.
- Add audit append behavior and prove a failed audit prevents the policy mutation.

### Phase 3 — evaluator and delegation service

- Implement actor context, exact scope matching, role expansion, deny precedence, compatibility baselines, and stable errors.
- Implement assignment, binding, and role commands with anti-escalation checks.
- Add table-driven evaluator and delegation tests before service integration.

### Phase 4 — composition and actor propagation

- Wire stores, registry, evaluator, identity/membership adapters, and cleanup.
- Attach actors in middleware without changing handler files.
- Mark and test every trusted system caller explicitly; missing actors fail closed.

### Phase 5 — first service enforcement

- Inject the narrow authorizer into project service.
- Enforce `projects.lifecycle.manage` and `projects.access.manage` at service entry points.
- Keep validation and domain invariants after authorization.
- Add direct service tests proving a call cannot bypass the decision by avoiding HTTP.

### Phase 6 — documentation and developer ergonomics

- Add a developer guide showing how to declare one definition, register it, inject `Authorizer`, choose a scope/baseline, and test allow/deny/admin/system behavior.
- Document persisted key stability and the required migration for a rename.
- Add an architecture rule/test that protected service methods do not call concrete auth, handlers, or stores.

After each phase, run focused package tests. At the end run:

```bash
cd backend
go test ./internal/service/permission/...
go test ./internal/stores/filepermissions/...
go test ./internal/service/project/...
go test ./internal/service/...
go test ./internal/transport/http/middleware/...
go test ./...
```

Because this changes service composition, persistence, and access policy, also run the repository's normal frontend build/tests and use `infra/qa/deploy-app.sh <pushed-ref>` on an existing QA installation. A full updater test is not required unless infrastructure or release inputs also change.

---

## Required test matrix

Registry:

- Accepts valid unique code definitions.
- Rejects duplicate/malformed keys and invalid scope/baseline combinations.
- Rejects persisted unknown keys at startup.

Evaluation:

- Default deny for `BaselineNone`.
- Project membership baseline allows current members.
- Direct allow works at the exact scope.
- Direct deny overrides direct allow, role allow, and compatibility baseline.
- A rule for project A does not match project B.
- Existing administrators remain allowed as root/break-glass actors.
- Missing actor fails closed; explicit system actor works only on reviewed internal paths.

Roles:

- Multiple permissions can be grouped and bound once.
- A role deny overrides another role's allow.
- Updating a role affects its bindings without rewriting users.
- Bound roles cannot be accidentally deleted.

Delegation:

- Admin may assign any registered permission.
- A delegated manager may assign a delegable permission they hold at the same scope.
- A manager cannot grant a permission or broader scope they do not hold.
- A manager cannot smuggle unauthorized keys through a custom role.
- Concurrent revocation and grant cannot pass a stale authorization check.
- Every successful mutation has an audit event; audit failure rejects mutation.

Integration:

- Existing member behavior is unchanged with an empty permission store.
- A project-scoped deny prevents lifecycle or access management even when HTTP membership checks pass.
- Direct service calls receive the same denial.
- Removing a user cleans their assignments and bindings.
- Restart reloads identical decisions from disk.

---

## Acceptance criteria

- Developers add a permission by declaring one validated definition in the owning service and registering it at composition.
- Runtime callers cannot create arbitrary permission keys.
- The same evaluator supports direct allow, direct deny, custom roles, and permission delegation.
- Deny precedence and scope matching are deterministic and covered by tests.
- Existing admins and project members retain current access when no explicit policy exists.
- The first two permissions are enforced inside project service methods, not only at HTTP edges.
- A call that bypasses handlers cannot bypass service authorization.
- User deletion leaves no assignments or bindings for that email.
- Permission changes are durable and audited.
- No handler file is changed in the initial delivery.

---

## Self-review

### Requirements check

- **No new admin/user layer:** satisfied. Existing users are referenced by email; existing administrators are the root policy.
- **Permission layer only:** satisfied. The new domain owns definitions, assignments, roles, evaluation, delegation, and audit.
- **Allow and deny:** satisfied with deterministic deny precedence.
- **Custom roles:** satisfied as persisted bundles independent of `user.Role`.
- **Permission to grant permissions:** satisfied by registered management permissions plus anti-escalation checks.
- **Developers alone add permission definitions:** satisfied by the immutable code registry and absence of a runtime create-definition operation.
- **Service-owned enforcement:** satisfied for the first two protected capabilities.
- **Developer ergonomics:** satisfied by owner-local definitions, a shared check type, one narrow authorizer port, and a documented recipe.
- **Registry pattern reviewed:** accepted for cataloging; explicitly rejected as the sole authorization mechanism.

### Security review

- Root admin is deliberately not mutable, preventing loss of the only recovery path.
- Explicit deny wins for non-admins, including over compatibility baselines.
- Delegators cannot grant capabilities they do not possess at the same scope.
- Missing actor is not treated as trusted internal work.
- Permission keys and scope kinds are validated, not arbitrary policy expressions.
- Store corruption and unknown keys fail loudly instead of weakening access.
- Audit failure blocks mutation.

### Architectural review

- The evaluator depends on narrow identity and membership ports.
- Owning services depend on a narrow authorizer, not auth, handlers, or persistence.
- Registry definitions live beside the service behavior they describe.
- File persistence remains an adapter.
- Context carries authenticated identity because handler signatures are frozen; it does not carry caller-supplied authorization facts such as `isAdmin`.

### Known limitation caused by the no-handler rule

The initial delivery cannot make delegated non-admins use routes that an existing handler rejects with a hard-coded `isAdmin` check. Examples include global application management, usage prices/rebuild, self-update, project deletion, and project container limits. Service authorization can make those operations safer, but it cannot broaden access when the handler rejects the request before calling the service.

Likewise, with no new or modified transport handler, custom-role and assignment commands are available as application-service APIs but are not yet manageable through the web UI/API. Claiming otherwise would be incorrect.

Therefore the no-handler delivery is a sound permission foundation and can enforce restrictions on routes that already reach their services, but a later explicitly authorized transport task is required for an administrator-facing permission-management API and for delegated access to currently admin-gated routes. That later handler work must remain mechanical: authenticate, decode, call the permission service, and map errors; it must not contain permission policy.

### Final decision

Proceed with the registry + evaluator + persisted policy design and the two project permissions. Do not attempt a repository-wide authorization rewrite. Do not claim end-to-end administrator management is complete until the separate transport constraint is relaxed.


---

## Progress

Tier: none recorded in the plan; treated as Standard.

- [x] Phase 0 characterization — `verified`: admin recognition tests (local and directory admin, member, unknown) and route tests (member/admin 200, non-member 403, anonymous 401) passed against unmodified code.
- [x] Phase 1 domain and registry — `verified`: `go test ./internal/service/permission/` (duplicate/malformed keys, bad scopes/baselines, immutability, resolve).
- [x] Phase 2 file store and audit — `verified`: `go test -race ./internal/stores/filepermissions/` (persist/reload, 0600/0700, failed audit leaves state untouched, 24 concurrent writers lose nothing, corruption and unknown ids fail startup).
- [x] Phase 3 evaluator and delegation — `verified`: table tests, delegation and anti-escalation tests, and a 200-iteration concurrent revoke/grant linearizability test under `-race`; two safeguards were mutated to confirm the tests fail without them.
- [x] Phase 4 composition and actor propagation — `verified`: middleware, cleanup, and restart tests; the real `cmd/remote` binary starts and listens with an empty policy, and stops with an actionable error for an unknown key and for a corrupt file.
- [x] Phase 5 first service enforcement — `verified`: project service and route/direct-call tests; the three system-actor markings were mutated to confirm tests fail without them.
- [x] Phase 6 documentation and architecture rules — `verified`: architecture tests pass and were shown to fail on a deliberate violation.
- Final gate — `verified`: every command in the plan's list passes, plus `go test -race` on the new packages, `go vet ./...`, `gofmt -l`, `npm test` (359 pass), and `npm run build`.
- `infra/qa/deploy-app.sh <pushed-ref>` — `blocked`: needs a QA installation and `.qa.env`, neither available here. Run it against the pushed branch to close this.

## Outcome

**Deviations from the plan:**

- `MustRegistry` and `NewRegistry` take `...[]Definition` (one group per owning service), not `...Definition`. The plan's example spread two slices into one variadic call, which is not valid Go; the groups form matches its intent. The composition root uses `NewRegistry` so a bad catalog returns an error from `services.New` instead of panicking.
- Auth is now built before the project service, over a second `serviceuser.New(deps.Users)` that has no removal cleanup. The plan anticipated this cycle (project -> authorizer -> identity -> auth -> user -> cleanup -> project) and allowed narrow adapters; the user service holds only its repository and the cleanup, so two instances over one repository are equivalent, and auth never removes users.
- Delegation is stricter than rule 4 as written. Rule 4 only checks role *binding*, which would let a non-administrator with `roles.manage` widen a role already bound to others by editing it. `UpdateRole` and `DeleteRole` (with unbind) now require the actor to be able to delegate every old and new rule at each existing binding's scope. This is the plan's own stated intent ("a manager cannot smuggle unauthorized keys through a custom role") and is covered by tests.
- Management permissions are declared non-delegable, so only administrators hand them out. Rule 5 still applies to role content.
- A project service built without an authorizer admits only the explicit system actor (fail closed), and five existing tests in `service_start_test.go` now pass an allow-all authorizer in setup. Their assertions are unchanged.
- The Phase 0 route test was rewritten in Phase 5 to run through the real middleware, permission service, and file store. It keeps the Phase 0 assertions and adds deny, direct-call, restart, and non-member cases.
- `ListAccess`, `AddAccess`, `RemoveAccess`, `Start`, `Stop`, `Restart`, `RepairNetwork` validate the project ID first, then authorize, then do lookups. This keeps `ErrInvalidID` meaning and stops a caller without permission learning whether a project exists.
- One extra hook, `Service.RemoveUserPolicy`, is the cleanup entry point for user removal. Read methods (`Assignments`, `Roles`, `Bindings`) require either management permission.

**Plan defects:**

- The plan never says how the evaluator reads state during a mutation without deadlocking on the store lock; the implementation makes evaluation pure over a snapshot and has `Repository.Mutate` hand the snapshot to the change function.
- "Explicit allow does not bypass independent resource visibility" is ambiguous at the evaluator. The evaluator honors an explicit allow for a non-member; visibility is still enforced by the handlers' membership check, which runs first (tested). A direct service call by a non-member holding an explicit allow succeeds.
- The plan does not mention that the handlers' `sendProjectError` has no mapping for `ErrDenied`, so a service denial returns `500 {"error":"permission denied"}` rather than `403`. Fixing it needs a handler change, which the plan freezes.

**Steps not fully verified:** `infra/qa/deploy-app.sh` (blocked, see Progress).

**Left behind:**

- Map `permission.ErrDenied` to `403` in the handlers, and add the administrator-facing permission-management routes and UI, in a separate explicitly authorized transport task.
- Routes with hard-coded `isAdmin` handler checks still cannot be opened to delegated non-administrators.
- Container starts through agent runs, the agent browser, and installed applications act under their own capability, so a member denied `projects.lifecycle.manage` can still cause a start that way.
- `docs/known-limitations.md` still says deleting a user does not sweep project access records; user removal already did that before this change, and now also removes permission policy. Not touched.

**For the next plan in this area:**

- Say how the evaluator and the repository share a snapshot, and state the default for a service built without an authorizer.
- Decide up front whether an explicit allow should require independent visibility at the evaluator or only at the handler.
- List the existing tests that call protected methods with a bare context; they all need an authorizer or actor once enforcement lands.
