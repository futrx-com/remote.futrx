export type RbacEffect = "allow" | "deny";
export type RbacScopeKind = "platform" | "project" | "provider-account";
export type RbacBaseline = "none" | "admin" | "project-member" | "authenticated";

export interface RbacScope {
  kind: RbacScopeKind;
  id?: string;
}

export interface RbacDefinition {
  key: string;
  description: string;
  scopes: RbacScopeKind[];
  baseline: RbacBaseline;
  delegable: boolean;
}

export interface RbacRoleRule {
  permission: string;
  effect: RbacEffect;
}

export interface RbacRole {
  id: string;
  name: string;
  description: string;
  rules: RbacRoleRule[];
  createdBy: string;
  createdAt: number;
  updatedAt: number;
}

export interface RbacAssignment {
  id: string;
  userEmail: string;
  permission: string;
  effect: RbacEffect;
  scope: RbacScope;
  createdBy: string;
  createdAt: number;
}

export interface RbacBinding {
  id: string;
  roleId: string;
  userEmail: string;
  scope: RbacScope;
  createdBy: string;
  createdAt: number;
}

export interface RbacRoleInput {
  name: string;
  description: string;
  rules: RbacRoleRule[];
}

export interface RbacAssignmentInput {
  userEmail: string;
  permission: string;
  effect: RbacEffect;
  scope: RbacScope;
}

export interface RbacAssignmentTarget {
  userEmail: string;
  permission: string;
  scope: RbacScope;
}

export interface RbacBindingInput {
  roleId: string;
  userEmail: string;
  scope: RbacScope;
}

export type RbacBindingTarget = RbacBindingInput;

export interface RbacAccountTarget { provider: string; accountId: string; label: string; scope: RbacScope; }

/** One mounted administration view; refreshed collections always arrive together. */
export interface PermissionAdministrationSnapshot {
  definitions: RbacDefinition[];
  roles: RbacRole[];
  assignments: RbacAssignment[];
  bindings: RbacBinding[];
}

export interface PermissionAdministrationState extends PermissionAdministrationSnapshot {
  loading: boolean;
  loaded: boolean;
  error: string | null;
}

/** `inherit` = no rule in the role; the permission falls back to its baseline. */
export type RuleState = "inherit" | "allow" | "deny";

export interface PermissionEntry {
  definition: RbacDefinition;
  /** Last segment of the key, e.g. `manage`. */
  action: string;
  state: RuleState;
  /** True when the state differs from what the role currently has saved. */
  customized: boolean;
  /** False when the role already holds it but the editor may not change it. */
  editable: boolean;
}

export interface PermissionCategory {
  name: string;
  entries: PermissionEntry[];
}

export interface PermissionBranch {
  id: string;
  categories: PermissionCategory[];
  total: number;
  allowed: number;
  denied: number;
  customized: number;
}

export type FormResult<T> = { ok: true; input: T } | { ok: false; message: string };

export interface AssignmentDraft {
  userEmail: string;
  permission: string;
  effect: RbacEffect;
  scopeKind: RbacScopeKind;
  projectId: string;
}

export interface BindingDraft {
  userEmail: string;
  roleId: string;
  scopeKind: RbacScopeKind;
  projectId: string;
}

export interface RoleDraft {
  name: string;
  description: string;
  rules: RbacRoleRule[];
}

export interface EffectivePermissionSnapshot {
  scope?: string;
  values: Record<string, boolean>;
}
