import assert from "node:assert/strict";
import test from "node:test";
import type { RbacDefinition, RbacRole } from "../../models/rbac.ts";
import { permissionFormService } from "./permissionFormService.ts";

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
  assert.deepEqual(permissionFormService.scopeKindsForPermission(definitions, "projects.access.manage"), ["project"]);
  assert.deepEqual(permissionFormService.scopeKindsForPermission(definitions, "nope.nope.nope"), []);
  assert.deepEqual(
    permissionFormService.scopeKindsForRole(definitions, role(["projects.both.manage", "projects.access.manage"])),
    ["project"]
  );
  assert.deepEqual(
    permissionFormService.scopeKindsForRole(definitions, role(["permissions.roles.manage", "projects.access.manage"])),
    []
  );
});

test("assignment validation normalizes email and builds the scope", () => {
  const ok = permissionFormService.validateAssignment(
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
  assert.deepEqual(permissionFormService.validateAssignment({ ...base, userEmail: "" }, definitions), { ok: false, message: "Email is required." });
  assert.deepEqual(permissionFormService.validateAssignment({ ...base, userEmail: "x" }, definitions), { ok: false, message: "That doesn't look like an email." });
  assert.deepEqual(permissionFormService.validateAssignment({ ...base, permission: "" }, definitions), { ok: false, message: "Choose a permission." });
  assert.deepEqual(permissionFormService.validateAssignment({ ...base, permission: "x.y.z" }, definitions), { ok: false, message: "That permission is not registered." });
  assert.equal(permissionFormService.validateAssignment(base, definitions).ok, true);
  const project = { ...base, permission: "projects.access.manage" };
  assert.deepEqual(permissionFormService.validateAssignment(project, definitions), { ok: false, message: "Choose a project." });
});

test("binding validation requires a role whose rules share the scope kind", () => {
  const roles = [role(["projects.access.manage"]), { ...role(["permissions.roles.manage", "projects.access.manage"]), id: "r2" }];
  const draft = { userEmail: "a@b.co", roleId: "r1", scopeKind: "platform" as const, projectId: "p1" };
  assert.deepEqual(permissionFormService.validateBinding(draft, roles, definitions), {
    ok: true,
    input: { userEmail: "a@b.co", roleId: "r1", scope: { kind: "project", id: "p1" } },
  });
  assert.deepEqual(permissionFormService.validateBinding({ ...draft, roleId: "r2" }, roles, definitions), {
    ok: false,
    message: "This role's rules share no scope kind.",
  });
  assert.deepEqual(permissionFormService.validateBinding({ ...draft, roleId: "zz" }, roles, definitions), { ok: false, message: "Choose a role." });
});

test("role validation mirrors the server rules", () => {
  const roles = [role(["projects.access.manage"])];
  const draft = { name: " Editor ", description: " d ", rules: [{ permission: "projects.access.manage", effect: "allow" as const }] };
  assert.deepEqual(permissionFormService.validateRole(draft, roles, definitions), {
    ok: true,
    input: { name: "Editor", description: "d", rules: draft.rules },
  });
  assert.deepEqual(permissionFormService.validateRole({ ...draft, name: " " }, roles, definitions), { ok: false, message: "Name is required." });
  assert.deepEqual(permissionFormService.validateRole({ ...draft, name: "x".repeat(81) }, roles, definitions), { ok: false, message: "Name must be at most 80 characters." });
  assert.deepEqual(permissionFormService.validateRole({ ...draft, rules: [] }, roles, definitions), { ok: false, message: "Add at least one rule." });
  assert.deepEqual(
    permissionFormService.validateRole({ ...draft, rules: [...draft.rules, ...draft.rules] }, roles, definitions),
    { ok: false, message: "projects.access.manage appears more than once." }
  );
  assert.deepEqual(
    permissionFormService.validateRole({ ...draft, rules: [{ permission: "a.b.c", effect: "deny" }] }, roles, definitions),
    { ok: false, message: "a.b.c is not a registered permission." }
  );
});

test("role names are unique case-insensitively except against the role being edited", () => {
  const roles = [{ ...role(["projects.access.manage"]), name: "Editor" }];
  const draft = { name: "editor", description: "", rules: [{ permission: "projects.access.manage", effect: "allow" as const }] };
  assert.equal(permissionFormService.validateRole(draft, roles, definitions).ok, false);
  assert.equal(permissionFormService.validateRole(draft, roles, definitions, "r1").ok, true);
});




test("provider-account grants preserve exact scope and reject malformed identifiers", () => {
  const definitions = [{ key: "agents.account.use", scopes: ["provider-account"], description: "Use account", baseline: "none", delegable: true }] as RbacDefinition[];
  const draft = {userEmail: "member@example.com", permission: "agents.account.use", effect: "allow" as const, scopeKind: "provider-account" as const, projectId: "codex:team"};
  const result = permissionFormService.validateAssignment(draft, definitions);
  assert.equal(result.ok, true);
  if (result.ok) assert.deepEqual(result.input.scope, {kind: "provider-account", id: "codex:team"});
  assert.equal(permissionFormService.validateAssignment({...draft, projectId:"../bad"}, definitions).ok, false);
});

for (const provider of ["minimax", "kimi", "antigravity"]) {
  test(`${provider} scopes work for direct assignments and account role bindings`, () => {
    const definitions: RbacDefinition[] = [{
      key: "agents.account.use", scopes: ["provider-account"],
      description: "Use account", baseline: "none", delegable: true,
    }];
    const accountRole = role(["agents.account.use"]);
    for (const accountId of ["default", "team-account"]) {
      const scope = { kind: "provider-account" as const, id: `${provider}:${accountId}` };
      assert.deepEqual(permissionFormService.validateAssignment({
        userEmail: "member@example.com", permission: "agents.account.use",
        effect: "allow", scopeKind: scope.kind, projectId: scope.id,
      }, definitions), {
        ok: true,
        input: { userEmail: "member@example.com", permission: "agents.account.use", effect: "allow", scope },
      });
      assert.deepEqual(permissionFormService.validateBinding({
        userEmail: "member@example.com", roleId: accountRole.id,
        scopeKind: scope.kind, projectId: scope.id,
      }, [accountRole], definitions), {
        ok: true,
        input: { userEmail: "member@example.com", roleId: accountRole.id, scope },
      });
    }
  });
}
