import type {
  EffectivePermissionSnapshot, PermissionAdministrationSnapshot, RbacAccountTarget,
  RbacAssignment, RbacAssignmentInput, RbacAssignmentTarget,
  RbacBinding, RbacBindingInput, RbacBindingTarget,
  RbacDefinition, RbacRole, RbacRoleInput,
} from "../models/rbac.ts";

/** Capabilities required by permission administration, independent of HTTP. */
export interface PermissionAdministrationApi {
  listDefinitions(): Promise<RbacDefinition[]>;
  listRoles(): Promise<RbacRole[]>;
  listAssignments(): Promise<RbacAssignment[]>;
  listBindings(): Promise<RbacBinding[]>;
  addAssignment(input: RbacAssignmentInput): Promise<RbacAssignment>;
  removeAssignment(target: RbacAssignmentTarget): Promise<void>;
  addBinding(input: RbacBindingInput): Promise<RbacBinding>;
  removeBinding(target: RbacBindingTarget): Promise<void>;
  createRole(input: RbacRoleInput): Promise<RbacRole>;
  updateRole(id: string, input: RbacRoleInput): Promise<RbacRole>;
  deleteRole(id: string, unbind: boolean): Promise<void>;
}

export interface PermissionAdministrationStateActions {
  beginRefresh(): void;
  receiveSnapshot(snapshot: PermissionAdministrationSnapshot): void;
  failRefresh(message: string): void;
  finishRefresh(): void;
}

export interface PermissionAdministrationStore {
  getState(): PermissionAdministrationStateActions;
}

export interface PermissionAdministrationCommands {
  refresh(): Promise<void>;
  addAssignment(input: RbacAssignmentInput): Promise<void>;
  removeAssignment(target: RbacAssignmentTarget): Promise<void>;
  addBinding(input: RbacBindingInput): Promise<void>;
  removeBinding(target: RbacBindingTarget): Promise<void>;
  createRole(input: RbacRoleInput): Promise<void>;
  updateRole(id: string, input: RbacRoleInput): Promise<void>;
  deleteRole(id: string, unbind: boolean): Promise<void>;
}

export interface PermissionLookupApi {
  effective(projectId?: string): Promise<Record<string, boolean>>;
  listAccountTargets(): Promise<RbacAccountTarget[]>;
}

export interface PermissionLookup {
  effectiveSnapshot(projectId?: string): Promise<EffectivePermissionSnapshot>;
  accountTargets(): Promise<RbacAccountTarget[]>;
}
