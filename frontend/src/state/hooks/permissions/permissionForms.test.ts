import assert from "node:assert/strict";
import test from "node:test";
import type { RbacDefinition, RbacRole } from "../../../models/rbac";
import { ApiError } from "../../../api/apiError.ts";
import { permissionForms } from "./permissionForms.ts";

const definitions: RbacDefinition[] = [
  { key: "permissions.roles.manage", description: "d", scopes: ["platform"], baseline: "none", delegable: false },
  { key: "projects.access.manage", description: "d", scopes: ["project"], baseline: "project-member", delegable: true },
  { key: "projects.both.manage", description: "d", scopes: ["platform", "project"], baseline: "none", delegable: true },
];
const role = (rules: string[]): RbacRole => ({
  id: "r1", name: "n", description: "", createdBy: "", createdAt: 0, updatedAt: 0,
  rules: rules.map((permission) => ({ permission, effect: "allow" as const })),
});

test("scope kinds come from the definitions, intersected across role rules", () => {
  assert.deepEqual(permissionForms.scopeKindsForPermission(definitions, "projects.access.manage"), ["project"]);
  assert.deepEqual(permissionForms.scopeKindsForPermission(definitions, "nope.nope.nope"), []);
  assert.deepEqual(
    permissionForms.scopeKindsForRole(definitions, role(["projects.both.manage", "projects.access.manage"])),
    ["project"]
  );
  assert.deepEqual(
    permissionForms.scopeKindsForRole(definitions, role(["permissions.roles.manage", "projects.access.manage"])),
    []
  );
});

test("assignment validation normalizes email and builds the scope", () => {
  const ok = permissionForms.validateAssignment(
    { userEmail: " A@B.co ", permission: "projects.access.manage", effect: "deny", scopeKind: "platform", projectId: "p1" },
    definitions
  );
  assert.deepEqual(ok, {
    ok: true,
    input: {
      userEmail: "a@b.co",
      permission: "projects.access.manage",
      effect: "deny",
      scope: { kind: "project", id: "p1" },
    },
  });
});

test("assignment validation reports the first problem", () => {
  const base = { userEmail: "a@b.co", permission: "permissions.roles.manage", effect: "allow" as const, scopeKind: "platform" as const, projectId: "" };
  assert.deepEqual(permissionForms.validateAssignment({ ...base, userEmail: "" }, definitions), { ok: false, message: "Email is required." });
  assert.deepEqual(permissionForms.validateAssignment({ ...base, userEmail: "x" }, definitions), { ok: false, message: "That doesn't look like an email." });
  assert.deepEqual(permissionForms.validateAssignment({ ...base, permission: "" }, definitions), { ok: false, message: "Choose a permission." });
  assert.deepEqual(permissionForms.validateAssignment({ ...base, permission: "x.y.z" }, definitions), { ok: false, message: "That permission is not registered." });
  assert.equal(permissionForms.validateAssignment(base, definitions).ok, true);
  const project = { ...base, permission: "projects.access.manage" };
  assert.deepEqual(permissionForms.validateAssignment(project, definitions), { ok: false, message: "Choose a project." });
});

test("binding validation requires a role whose rules share the scope kind", () => {
  const roles = [role(["projects.access.manage"]), { ...role(["permissions.roles.manage", "projects.access.manage"]), id: "r2" }];
  const draft = { userEmail: "a@b.co", roleId: "r1", scopeKind: "platform" as const, projectId: "p1" };
  assert.deepEqual(permissionForms.validateBinding(draft, roles, definitions), {
    ok: true,
    input: { userEmail: "a@b.co", roleId: "r1", scope: { kind: "project", id: "p1" } },
  });
  assert.deepEqual(permissionForms.validateBinding({ ...draft, roleId: "r2" }, roles, definitions), {
    ok: false,
    message: "This role's rules share no scope kind.",
  });
  assert.deepEqual(permissionForms.validateBinding({ ...draft, roleId: "zz" }, roles, definitions), { ok: false, message: "Choose a role." });
});

test("role validation mirrors the server rules", () => {
  const roles = [role(["projects.access.manage"])];
  const draft = { name: " Editor ", description: " d ", rules: [{ permission: "projects.access.manage", effect: "allow" as const }] };
  assert.deepEqual(permissionForms.validateRole(draft, roles, definitions), {
    ok: true,
    input: { name: "Editor", description: "d", rules: draft.rules },
  });
  assert.deepEqual(permissionForms.validateRole({ ...draft, name: " " }, roles, definitions), { ok: false, message: "Name is required." });
  assert.deepEqual(permissionForms.validateRole({ ...draft, name: "x".repeat(81) }, roles, definitions), { ok: false, message: "Name must be at most 80 characters." });
  assert.deepEqual(permissionForms.validateRole({ ...draft, rules: [] }, roles, definitions), { ok: false, message: "Add at least one rule." });
  assert.deepEqual(
    permissionForms.validateRole({ ...draft, rules: [...draft.rules, ...draft.rules] }, roles, definitions),
    { ok: false, message: "projects.access.manage appears more than once." }
  );
  assert.deepEqual(
    permissionForms.validateRole({ ...draft, rules: [{ permission: "a.b.c", effect: "deny" }] }, roles, definitions),
    { ok: false, message: "a.b.c is not a registered permission." }
  );
});

test("role names are unique case-insensitively except against the role being edited", () => {
  const roles = [{ ...role(["projects.access.manage"]), name: "Editor" }];
  const draft = { name: "editor", description: "", rules: [{ permission: "projects.access.manage", effect: "allow" as const }] };
  assert.equal(permissionForms.validateRole(draft, roles, definitions).ok, false);
  assert.equal(permissionForms.validateRole(draft, roles, definitions, "r1").ok, true);
});

test("a 409 on delete offers the unbind option; other errors pass through", () => {
  const conflict = new ApiError("role is bound to users", 409);
  assert.equal(permissionForms.isRoleStillBound(conflict), true);
  assert.match(permissionForms.deleteRoleErrorMessage(conflict), /Also remove its bindings/);
  const other = new ApiError("boom", 500);
  assert.equal(permissionForms.isRoleStillBound(other), false);
  assert.equal(permissionForms.deleteRoleErrorMessage(other), "boom");
});
