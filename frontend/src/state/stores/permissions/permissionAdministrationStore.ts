import { createStore } from "zustand/vanilla";
import type { PermissionAdministrationState } from "../../../models/rbac.ts";
import type { PermissionAdministrationStateActions } from "../../../port/permissions.ts";

/** Per-mount view data. Failed refreshes retain the last successfully loaded collections. */
export function createPermissionAdministrationStore() {
  return createStore<PermissionAdministrationState & PermissionAdministrationStateActions>()((set) => ({
    definitions: [],
    roles: [],
    assignments: [],
    bindings: [],
    loading: false,
    loaded: false,
    error: null,
    beginRefresh: () => set({ loading: true, error: null }),
    receiveSnapshot: (snapshot) => set({ ...snapshot, loaded: true }),
    failRefresh: (error) => set({ error }),
    finishRefresh: () => set({ loading: false }),
  }));
}
