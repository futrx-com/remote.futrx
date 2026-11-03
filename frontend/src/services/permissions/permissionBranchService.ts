import { DEFAULT_PERMISSION_CATEGORY, PERMISSION_BASELINE_LABELS } from "../../config/constants/permissions.ts";
import type {
  PermissionBranch, PermissionEntry, RuleState, RbacDefinition, RbacRoleRule,
} from "../../models/rbac.ts";

class PermissionBranchService {
  /**
   * The single policy for what a role editor may hand out, mirroring the
   * server's delegation rule: only delegable permissions can be added.
   */
  private isGrantable(definition: RbacDefinition, grantAll: boolean): boolean {
    return grantAll || definition.delegable;
  }

  /** Registered baselines get friendly copy; one added later still renders as its own name. */
  baselineLabel(baseline: string): string {
    return PERMISSION_BASELINE_LABELS[baseline] ?? baseline;
  }

  private stateOf(rules: RbacRoleRule[], key: string): RuleState {
    return rules.find((rule) => rule.permission === key)?.effect ?? "inherit";
  }

  /** Returns new rules with `key` set to `state`; `inherit` removes the rule. */
  withState(rules: RbacRoleRule[], key: string, state: RuleState): RbacRoleRule[] {
    const rest = rules.filter((rule) => rule.permission !== key);
    return state === "inherit" ? rest : [...rest, { permission: key, effect: state }];
  }

  /** Sets every editable entry of `branch` to `state`, leaving locked ones untouched. */
  withBranchState(rules: RbacRoleRule[], branch: PermissionBranch, state: RuleState): RbacRoleRule[] {
    return branch.categories
      .flatMap((category) => category.entries)
      .filter((entry) => entry.editable)
      .reduce((next, entry) => this.withState(next, entry.definition.key, state), rules);
  }

  /**
   * Whether `rule` may be written: grantable permissions always, others only
   * when the saved role already carries the identical rule.
   */
  canWrite(
    definition: RbacDefinition,
    rule: RbacRoleRule,
    savedRules: RbacRoleRule[],
    grantAll: boolean
  ): boolean {
    return (
      this.isGrantable(definition, grantAll) ||
      savedRules.some((saved) => saved.permission === rule.permission && saved.effect === rule.effect)
    );
  }

  /**
   * Groups permissions into branch → category → entry. Permissions the editor
   * cannot grant and the role does not already hold are left out entirely.
   */
  buildBranches(
    definitions: RbacDefinition[],
    rules: RbacRoleRule[],
    savedRules: RbacRoleRule[],
    grantAll: boolean
  ): PermissionBranch[] {
    const branches = new Map<string, Map<string, PermissionEntry[]>>();
    for (const definition of definitions) {
      const state = this.stateOf(rules, definition.key);
      const saved = this.stateOf(savedRules, definition.key);
      const grantable = this.isGrantable(definition, grantAll);
      if (!grantable && saved === "inherit") continue;
      const [branchId, ...rest] = definition.key.split(".");
      const action = rest.pop() ?? definition.key;
      const category = rest.join(".") || DEFAULT_PERMISSION_CATEGORY;
      const categories = branches.get(branchId) ?? new Map<string, PermissionEntry[]>();
      const entries = categories.get(category) ?? [];
      entries.push({ definition, action, state, customized: state !== saved, editable: grantable });
      categories.set(category, entries);
      branches.set(branchId, categories);
    }
    return [...branches].map(([id, categories]) => this.summarize(id, categories));
  }

  private summarize(id: string, categories: Map<string, PermissionEntry[]>): PermissionBranch {
    const groups = [...categories].map(([name, entries]) => ({ name, entries }));
    const all = groups.flatMap((group) => group.entries);
    return {
      id,
      categories: groups,
      total: all.length,
      allowed: all.filter((entry) => entry.state === "allow").length,
      denied: all.filter((entry) => entry.state === "deny").length,
      customized: all.filter((entry) => entry.customized).length,
    };
  }
}

export const permissionBranchService = new PermissionBranchService();
