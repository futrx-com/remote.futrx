import { ApiError } from "../../../api/apiError.ts";
import type {
  RbacAssignmentInput,
  RbacBindingInput,
  RbacDefinition,
  RbacEffect,
  RbacRole,
  RbacRoleInput,
  RbacRoleRule,
  RbacScope,
  RbacScopeKind,
} from "../../../models/rbac";
import { permissionBranches } from "./permissionBranches.ts";

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

// Mirrors the limits in backend/internal/rbac/roles.go.
const ROLE_NAME_MAX = 80;
const ROLE_DESCRIPTION_MAX = 500;

const EMAIL_PATTERN = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

class PermissionFormLogic {
  /** Scope kinds the permission is registered for; the definitions list is the only source. */
  scopeKindsForPermission(definitions: RbacDefinition[], key: string): RbacScopeKind[] {
    return definitions.find((definition) => definition.key === key)?.scopes ?? [];
  }

  /** Scope kinds every rule of the role supports, mirroring the server's bind-time check. */
  scopeKindsForRole(definitions: RbacDefinition[], role: RbacRole | undefined): RbacScopeKind[] {
    if (!role || role.rules.length === 0) return [];
    const [first, ...rest] = role.rules.map((rule) =>
      this.scopeKindsForPermission(definitions, rule.permission)
    );
    return first.filter((kind) => rest.every((kinds) => kinds.includes(kind)));
  }

  /** The kind to show: the chosen one when still offered, else the first offered. */
  effectiveScopeKind(offered: RbacScopeKind[], chosen: RbacScopeKind): RbacScopeKind {
    return offered.includes(chosen) ? chosen : offered[0] ?? chosen;
  }

  buildScope(kind: RbacScopeKind, projectId: string): RbacScope {
    return kind === "project" ? { kind, id: projectId } : { kind };
  }

  validateAssignment(
    draft: AssignmentDraft,
    definitions: RbacDefinition[]
  ): FormResult<RbacAssignmentInput> {
    const userEmail = draft.userEmail.trim().toLowerCase();
    if (!userEmail) return { ok: false, message: "Email is required." };
    if (!EMAIL_PATTERN.test(userEmail)) return { ok: false, message: "That doesn't look like an email." };
    if (!draft.permission) return { ok: false, message: "Choose a permission." };
    const offered = this.scopeKindsForPermission(definitions, draft.permission);
    const kind = this.effectiveScopeKind(offered, draft.scopeKind);
    if (!offered.includes(kind)) return { ok: false, message: "That permission is not registered." };
    if (kind === "project" && !draft.projectId) return { ok: false, message: "Choose a project." };
    return {
      ok: true,
      input: {
        userEmail,
        permission: draft.permission,
        effect: draft.effect,
        scope: this.buildScope(kind, draft.projectId),
      },
    };
  }

  validateBinding(
    draft: BindingDraft,
    roles: RbacRole[],
    definitions: RbacDefinition[]
  ): FormResult<RbacBindingInput> {
    const userEmail = draft.userEmail.trim().toLowerCase();
    if (!userEmail) return { ok: false, message: "Email is required." };
    if (!EMAIL_PATTERN.test(userEmail)) return { ok: false, message: "That doesn't look like an email." };
    const role = roles.find((candidate) => candidate.id === draft.roleId);
    if (!role) return { ok: false, message: "Choose a role." };
    const offered = this.scopeKindsForRole(definitions, role);
    const kind = this.effectiveScopeKind(offered, draft.scopeKind);
    if (!offered.includes(kind)) {
      return { ok: false, message: "This role's rules share no scope kind." };
    }
    if (kind === "project" && !draft.projectId) return { ok: false, message: "Choose a project." };
    return {
      ok: true,
      input: { userEmail, roleId: role.id, scope: this.buildScope(kind, draft.projectId) },
    };
  }

  /**
   * `selfId` is the role being edited, so it does not collide with its own name.
   * `grantAll` is true for administrators, who may grant non-delegable permissions.
   */
  validateRole(
    draft: RoleDraft,
    roles: RbacRole[],
    definitions: RbacDefinition[],
    selfId?: string,
    grantAll = false
  ): FormResult<RbacRoleInput> {
    const name = draft.name.trim();
    const description = draft.description.trim();
    if (!name) return { ok: false, message: "Name is required." };
    if (name.length > ROLE_NAME_MAX) {
      return { ok: false, message: `Name must be at most ${ROLE_NAME_MAX} characters.` };
    }
    if (description.length > ROLE_DESCRIPTION_MAX) {
      return { ok: false, message: `Description must be at most ${ROLE_DESCRIPTION_MAX} characters.` };
    }
    const taken = roles.some(
      (role) => role.id !== selfId && role.name.trim().toLowerCase() === name.toLowerCase()
    );
    if (taken) return { ok: false, message: `A role named ${name} already exists.` };
    if (draft.rules.length === 0) return { ok: false, message: "Add at least one rule." };
    const seen = new Set<string>();
    const savedRules = roles.find((role) => role.id === selfId)?.rules ?? [];
    for (const rule of draft.rules) {
      if (!rule.permission) return { ok: false, message: "Choose a permission for every rule." };
      const definition = definitions.find((candidate) => candidate.key === rule.permission);
      if (!definition) {
        return { ok: false, message: `${rule.permission} is not a registered permission.` };
      }
      if (!permissionBranches.canWrite(definition, rule, savedRules, grantAll)) {
        return { ok: false, message: `${rule.permission} cannot be granted from here.` };
      }
      if (seen.has(rule.permission)) {
        return { ok: false, message: `${rule.permission} appears more than once.` };
      }
      seen.add(rule.permission);
    }
    return { ok: true, input: { name, description, rules: draft.rules } };
  }

  /** A 409 means the role is still bound; every other failure is reported as the server sent it. */
  deleteRoleErrorMessage(cause: unknown): string {
    if (cause instanceof ApiError && cause.status === 409) {
      return "This role is still bound to users. Tick “Also remove its bindings” to delete it anyway.";
    }
    return (cause as Error).message || "Something went wrong.";
  }

  isRoleStillBound(cause: unknown): boolean {
    return cause instanceof ApiError && cause.status === 409;
  }
}

export const permissionForms = new PermissionFormLogic();
