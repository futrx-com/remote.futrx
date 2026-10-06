import type { UserRole } from "../../models/user";
import type { RbacScope } from "../../models/rbac";

// Every base role the application supports (mirrors the UserRole type).
export const USER_ROLES: UserRole[] = ["member", "admin"];

export const DEFAULT_USER_ROLE: UserRole = "member";

// Scope at which custom roles are bound from the user directory.
export const PLATFORM_SCOPE: RbacScope = { kind: "platform" };
