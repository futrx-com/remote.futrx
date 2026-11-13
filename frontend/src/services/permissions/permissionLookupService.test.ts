import assert from "node:assert/strict";
import test from "node:test";
import type { RbacAccountTarget } from "../../models/rbac.ts";
import { PermissionLookupService } from "./permissionLookupService.ts";

test("permission hints keep their scope and account suggestions retain server order", async () => {
  const values = { "projects.create": true };
  const accounts: RbacAccountTarget[] = [
    { provider: "codex", accountId: "team", label: "Team", scope: { kind: "provider-account", id: "codex:team" } },
    { provider: "minimax", accountId: "default", label: "Default", scope: { kind: "provider-account", id: "minimax:default" } },
    { provider: "kimi", accountId: "team", label: "Team", scope: { kind: "provider-account", id: "kimi:team" } },
    { provider: "antigravity", accountId: "default", label: "Default", scope: { kind: "provider-account", id: "antigravity:default" } },
  ];
  const service = new PermissionLookupService({
    effective: async (projectId) => { assert.equal(projectId, "p1"); return values; },
    listAccountTargets: async () => accounts,
  });
  assert.deepEqual(await service.effectiveSnapshot("p1"), { scope: "p1", values });
  assert.equal(await service.accountTargets(), accounts);
});

test("failed permission reads hide actions and failed account reads clear suggestions", async () => {
  const service = new PermissionLookupService({
    effective: async () => { throw new Error("unavailable"); },
    listAccountTargets: async () => { throw new Error("unavailable"); },
  });
  assert.deepEqual(await service.effectiveSnapshot("p1"), { scope: "p1", values: {} });
  assert.deepEqual(await service.effectiveSnapshot(), { scope: undefined, values: {} });
  assert.deepEqual(await service.accountTargets(), []);
});
