import assert from "node:assert/strict";
import test from "node:test";
import type { PermissionAdministrationApi } from "../../port/permissions.ts";
import { createPermissionAdministrationStore } from "../../state/stores/permissions/permissionAdministrationStore.ts";
import { PermissionAdministrationService } from "./permissionAdministrationService.ts";

const role = { id: "r1", name: "Editor", description: "", rules: [], createdBy: "", createdAt: 0, updatedAt: 0 };

function setup() {
  const calls: string[] = [];
  const api: PermissionAdministrationApi = {
    listDefinitions: async () => { calls.push("definitions"); return []; },
    listRoles: async () => { calls.push("roles"); return [role]; },
    listAssignments: async () => { calls.push("assignments"); return []; },
    listBindings: async () => { calls.push("bindings"); return []; },
    addAssignment: async () => { throw new Error("unused"); },
    removeAssignment: async () => {},
    addBinding: async () => { throw new Error("unused"); },
    removeBinding: async () => {},
    createRole: async () => role,
    updateRole: async () => role,
    deleteRole: async () => { calls.push("delete"); },
  };
  const store = createPermissionAdministrationStore();
  const service = new PermissionAdministrationService(api, store, () => calls.push("notify"));
  return { api, store, service, calls };
}

test("administration starts all reads in order and publishes the complete snapshot", async () => {
  const { service, store, calls } = setup();
  const pending = service.refresh();
  assert.equal(store.getState().loading, true);
  assert.equal(store.getState().loaded, false);
  assert.deepEqual(calls, ["definitions", "roles", "assignments", "bindings"]);
  await pending;
  assert.deepEqual(store.getState().roles, [role]);
  assert.equal(store.getState().loaded, true);
  assert.equal(store.getState().loading, false);
});

test("failed refresh retains loaded data, exposes the error and resolves", async () => {
  const { api, service, store } = setup();
  await service.refresh();
  const roles = store.getState().roles;
  api.listRoles = async () => { throw new Error("refresh failed"); };
  await service.refresh();
  assert.equal(store.getState().roles, roles);
  assert.equal(store.getState().loaded, true);
  assert.equal(store.getState().error, "refresh failed");
  assert.equal(store.getState().loading, false);
});

test("successful mutation notifies once before reloading and forwards arguments", async () => {
  const { api, service, calls } = setup();
  api.deleteRole = async (id, unbind) => {
    assert.equal(id, "r1");
    assert.equal(unbind, true);
    calls.push("delete");
  };
  await service.deleteRole("r1", true);
  assert.deepEqual(calls, ["delete", "notify", "definitions", "roles", "assignments", "bindings"]);
});

test("mutation rejection is returned unchanged without notification or reload", async () => {
  const { api, service, calls } = setup();
  const failure = new Error("role is bound");
  api.deleteRole = async () => { throw failure; };
  await assert.rejects(service.deleteRole("r1", false), (cause) => cause === failure);
  assert.deepEqual(calls, []);
});

test("each administration surface has isolated state", async () => {
  const first = setup();
  const second = setup();
  await first.service.refresh();
  assert.equal(first.store.getState().loaded, true);
  assert.equal(second.store.getState().loaded, false);
  assert.deepEqual(second.store.getState().roles, []);
});
