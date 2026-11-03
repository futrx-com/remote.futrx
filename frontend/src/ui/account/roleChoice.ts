import type { RbacBinding, RbacRole } from "../../models/rbac";
import type { UserRole } from "../../models/user";

// A selectable role: either a base role or a custom permission role.
export type RoleChoice =
  | { kind: "base"; role: UserRole }
  | { kind: "custom"; roleId: string };

const BASE_PREFIX = "base:";
const CUSTOM_PREFIX = "custom:";

export function encodeRoleChoice(choice: RoleChoice): string {
  return choice.kind === "base"
    ? BASE_PREFIX + choice.role
    : CUSTOM_PREFIX + choice.roleId;
}

export function decodeRoleChoice(value: string): RoleChoice {
  return value.startsWith(CUSTOM_PREFIX)
    ? { kind: "custom", roleId: value.slice(CUSTOM_PREFIX.length) }
    : { kind: "base", role: value.slice(BASE_PREFIX.length) as UserRole };
}

// Custom roles bound to the user at platform scope, in binding order.
export function boundCustomRoles(
  bindings: RbacBinding[],
  roles: RbacRole[],
  email: string
): RbacRole[] {
  const found: RbacRole[] = [];
  for (const binding of bindings) {
    if (binding.userEmail !== email || binding.scope.kind !== "platform") continue;
    const role = roles.find((candidate) => candidate.id === binding.roleId);
    if (role) found.push(role);
  }
  return found;
}
