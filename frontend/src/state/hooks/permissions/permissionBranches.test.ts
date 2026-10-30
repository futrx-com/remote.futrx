import assert from "node:assert/strict";
import test from "node:test";
import type { RbacDefinition } from "../../../models/rbac";
import { permissionForms } from "./permissionForms.ts";
import { permissionBranches } from "./permissionBranches.ts";

const def = (key: string, delegable: boolean): RbacDefinition => ({
  key, description: "d", scopes: ["platform"], baseline: "none", delegable,
});
const definitions = [
  def("projects.access.manage", true),
  def("projects.access.view", true),
  def("projects.files.read", true),
  def("permissions.roles.manage", false),
  def("permissions.assignments.manage", false),
];

test("branches group by context then resource and hide ungrantable permissions", () => {
  const branches = permissionBranches.buildBranches(definitions, [], [], false);
  assert.deepEqual(branches.map((branch) => branch.id), ["projects"]);
  assert.deepEqual(branches[0].categories.map((category) => category.name), ["access", "files"]);
  assert.equal(branches[0].total, 3);
});

test("an ungrantable permission the role already holds stays visible but locked", () => {
  const saved = [{ permission: "permissions.roles.manage", effect: "allow" as const }];
  const branches = permissionBranches.buildBranches(definitions, saved, saved, false);
  const entry = branches.find((branch) => branch.id === "permissions")?.categories[0].entries[0];
  assert.equal(entry?.editable, false);
  assert.equal(entry?.state, "allow");
});

test("states, customization counts and bulk updates", () => {
  const rules = permissionBranches.withState([], "projects.access.manage", "allow");
  const [branch] = permissionBranches.buildBranches(definitions, rules, [], false);
  assert.deepEqual([branch.allowed, branch.denied, branch.customized], [1, 0, 1]);
  const denied = permissionBranches.withBranchState(rules, branch, "deny");
  assert.equal(denied.length, 3);
  assert.equal(permissionBranches.withBranchState(denied, branch, "inherit").length, 0);
});

test("role validation refuses to add a permission the editor cannot grant", () => {
  const draft = { name: "r", description: "", rules: [{ permission: "permissions.roles.manage", effect: "allow" as const }] };
  assert.deepEqual(permissionForms.validateRole(draft, [], definitions), {
    ok: false,
    message: "permissions.roles.manage cannot be granted from here.",
  });
  const existing = { id: "r1", name: "x", description: "", createdBy: "", createdAt: 0, updatedAt: 0, rules: draft.rules };
  assert.equal(permissionForms.validateRole(draft, [existing], definitions, "r1").ok, true);
});

test("administrators can grant every permission", () => {
  const branches = permissionBranches.buildBranches(definitions, [], [], true);
  assert.deepEqual(branches.map((branch) => branch.id), ["projects", "permissions"]);
  const draft = { name: "r", description: "", rules: [{ permission: "permissions.roles.manage", effect: "allow" as const }] };
  assert.equal(permissionForms.validateRole(draft, [], definitions, undefined, true).ok, true);
});

test("permissions registered later (e.g. by a plugin) appear without code changes", () => {
  const plugin = [
    { ...def("billing.invoices.export", true), baseline: "future-baseline" as never },
    def("billing.invoices.void.force", true),
    def("billing", true),
  ];
  const [branch] = permissionBranches.buildBranches(plugin, [], [], false);
  assert.equal(branch.id, "billing");
  assert.deepEqual(branch.categories.map((category) => category.name).sort(), ["general", "invoices", "invoices.void"]);
  assert.equal(permissionBranches.baselineLabel("future-baseline"), "future-baseline");
  assert.equal(permissionBranches.baselineLabel("none"), "no default access");
});
