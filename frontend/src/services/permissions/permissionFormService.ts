import { ROLE_NAME_MAX, ROLE_DESCRIPTION_MAX } from "../../config/constants/permissions.ts";
import type {
  FormResult, AssignmentDraft, BindingDraft, RoleDraft,
  RbacAssignmentInput,
  RbacBindingInput,
  RbacDefinition,
  RbacRole,
  RbacRoleInput,
  RbacScope,
  RbacScopeKind,
} from "../../models/rbac.ts";
import { permissionBranchService } from "./permissionBranchService.ts";

const EMAIL_PATTERN = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

class PermissionFormService {
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

  private scopeError(kind: RbacScopeKind, id: string): string | null {
    if (kind === "provider-account" && !/^[a-z0-9][a-z0-9_-]{0,63}:[a-z0-9][a-z0-9_-]{0,63}$/.test(id)) {
      return "Enter a provider:account scope, such as claude:default.";
    }
    if (kind === "project" && !id) return "Choose a project.";
    return null;
  }

  private buildScope(kind: RbacScopeKind, projectId: string): RbacScope {
    return kind === "platform" ? { kind } : { kind, id: projectId };
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
    const scopeError = this.scopeError(kind, draft.projectId);
    if (scopeError) return { ok: false, message: scopeError };
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
    const scopeError = this.scopeError(kind, draft.projectId);
    if (scopeError) return { ok: false, message: scopeError };
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
      if (!permissionBranchService.canWrite(definition, rule, savedRules, grantAll)) {
        return { ok: false, message: `${rule.permission} cannot be granted from here.` };
      }
      if (seen.has(rule.permission)) {
        return { ok: false, message: `${rule.permission} appears more than once.` };
      }
      seen.add(rule.permission);
    }
    return { ok: true, input: { name, description, rules: draft.rules } };
  }


}

export const permissionFormService = new PermissionFormService();
