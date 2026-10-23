import { requestJson } from "./apiRequest";
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
} from "../models/rbac";
import { API_ROUTES } from "../config/routes";

// Mirrors api/permissions.contract.md. Every route here is a proposal until
// the backend implements it.
export const permissionsApi = {
  listDefinitions: async () =>
    (await requestJson<{ definitions: RbacDefinition[] }>("GET", API_ROUTES.permissions.definitions))
      .definitions,
  listRoles: async () =>
    (await requestJson<{ roles: RbacRole[] }>("GET", API_ROUTES.permissions.roles)).roles,
  listAssignments: async () =>
    (await requestJson<{ assignments: RbacAssignment[] }>("GET", API_ROUTES.permissions.assignments))
      .assignments,
  listBindings: async () =>
    (await requestJson<{ bindings: RbacBinding[] }>("GET", API_ROUTES.permissions.bindings)).bindings,
  addAssignment: (input: RbacAssignmentInput) =>
    requestJson<RbacAssignment>("POST", API_ROUTES.permissions.assignments, input),
  removeAssignment: (target: RbacAssignmentTarget) =>
    requestJson<void>("DELETE", API_ROUTES.permissions.assignmentTarget(target)),
  addBinding: (input: RbacBindingInput) =>
    requestJson<RbacBinding>("POST", API_ROUTES.permissions.bindings, input),
  removeBinding: (target: RbacBindingTarget) =>
    requestJson<void>("DELETE", API_ROUTES.permissions.bindingTarget(target)),
  createRole: (input: RbacRoleInput) =>
    requestJson<RbacRole>("POST", API_ROUTES.permissions.roles, input),
  updateRole: (id: string, input: RbacRoleInput) =>
    requestJson<RbacRole>("PUT", API_ROUTES.permissions.role(id), input),
  deleteRole: (id: string, unbind: boolean) =>
    requestJson<void>("DELETE", API_ROUTES.permissions.role(id, unbind)),
};
