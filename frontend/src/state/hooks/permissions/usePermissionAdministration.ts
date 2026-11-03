import { useEffect } from "preact/hooks";
import { useStore } from "zustand";
import type { StoreApi } from "zustand/vanilla";
import type { PermissionAdministrationState } from "../../../models/rbac";
import type {
  PermissionAdministrationCommands, PermissionAdministrationStateActions,
} from "../../../port/permissions";

export interface PermissionsController
  extends PermissionAdministrationState, PermissionAdministrationCommands {}

export function usePermissionAdministration(
  enabled: boolean,
  administration: {
    store: StoreApi<PermissionAdministrationState & PermissionAdministrationStateActions>;
    commands: PermissionAdministrationCommands;
  },
): PermissionsController {
  const { store, commands } = administration;
  const loading = useStore(store, (state) => state.loading);
  const loaded = useStore(store, (state) => state.loaded);
  const definitions = useStore(store, (state) => state.definitions);
  const roles = useStore(store, (state) => state.roles);
  const assignments = useStore(store, (state) => state.assignments);
  const bindings = useStore(store, (state) => state.bindings);
  const error = useStore(store, (state) => state.error);
  const {
    refresh, addAssignment, removeAssignment, addBinding, removeBinding,
    createRole, updateRole, deleteRole,
  } = commands;

  useEffect(() => {
    if (enabled) void refresh();
  }, [enabled, refresh]);

  return {
    loading, loaded, definitions, roles, assignments, bindings, error,
    refresh, addAssignment, removeAssignment, addBinding, removeBinding,
    createRole, updateRole, deleteRole,
  };
}
