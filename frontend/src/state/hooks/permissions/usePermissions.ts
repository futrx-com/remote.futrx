import { useCallback, useEffect, useState } from "preact/hooks";
import { permissionsApi } from "../../../api/permissions";
import type {
  RbacAssignment,
  RbacAssignmentInput,
  RbacAssignmentTarget,
  RbacBinding,
  RbacBindingInput,
  RbacBindingTarget,
  RbacDefinition,
  RbacRole,
  RbacRoleInput,
} from "../../../models/rbac";

export interface PermissionsController {
  loading: boolean;
  loaded: boolean;
  definitions: RbacDefinition[];
  roles: RbacRole[];
  assignments: RbacAssignment[];
  bindings: RbacBinding[];
  error: string | null;
  refresh: () => Promise<void>;
  addAssignment: (input: RbacAssignmentInput) => Promise<void>;
  removeAssignment: (target: RbacAssignmentTarget) => Promise<void>;
  addBinding: (input: RbacBindingInput) => Promise<void>;
  removeBinding: (target: RbacBindingTarget) => Promise<void>;
  createRole: (input: RbacRoleInput) => Promise<void>;
  updateRole: (id: string, input: RbacRoleInput) => Promise<void>;
  deleteRole: (id: string, unbind: boolean) => Promise<void>;
}

export function usePermissions(enabled: boolean): PermissionsController {
  const [loading, setLoading] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [definitions, setDefinitions] = useState<RbacDefinition[]>([]);
  const [roles, setRoles] = useState<RbacRole[]>([]);
  const [assignments, setAssignments] = useState<RbacAssignment[]>([]);
  const [bindings, setBindings] = useState<RbacBinding[]>([]);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [nextDefinitions, nextRoles, nextAssignments, nextBindings] = await Promise.all([
        permissionsApi.listDefinitions(),
        permissionsApi.listRoles(),
        permissionsApi.listAssignments(),
        permissionsApi.listBindings(),
      ]);
      setDefinitions(nextDefinitions);
      setRoles(nextRoles);
      setAssignments(nextAssignments);
      setBindings(nextBindings);
      setLoaded(true);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (enabled) void refresh();
  }, [enabled, refresh]);

  const addAssignment = useCallback(
    async (input: RbacAssignmentInput) => {
      await permissionsApi.addAssignment(input);
      await refresh();
    },
    [refresh]
  );

  const removeAssignment = useCallback(
    async (target: RbacAssignmentTarget) => {
      await permissionsApi.removeAssignment(target);
      await refresh();
    },
    [refresh]
  );

  const addBinding = useCallback(
    async (input: RbacBindingInput) => {
      await permissionsApi.addBinding(input);
      await refresh();
    },
    [refresh]
  );

  const removeBinding = useCallback(
    async (target: RbacBindingTarget) => {
      await permissionsApi.removeBinding(target);
      await refresh();
    },
    [refresh]
  );

  const createRole = useCallback(
    async (input: RbacRoleInput) => {
      await permissionsApi.createRole(input);
      await refresh();
    },
    [refresh]
  );

  const updateRole = useCallback(
    async (id: string, input: RbacRoleInput) => {
      await permissionsApi.updateRole(id, input);
      await refresh();
    },
    [refresh]
  );

  const deleteRole = useCallback(
    async (id: string, unbind: boolean) => {
      await permissionsApi.deleteRole(id, unbind);
      await refresh();
    },
    [refresh]
  );

  return {
    loading,
    loaded,
    definitions,
    roles,
    assignments,
    bindings,
    error,
    refresh,
    addAssignment,
    removeAssignment,
    addBinding,
    removeBinding,
    createRole,
    updateRole,
    deleteRole,
  };
}
