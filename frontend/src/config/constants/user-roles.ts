import type { UserRole } from "../../models/user";
import type { RbacScope } from "../../models/rbac";

// Every base role the application supports (mirrors the UserRole type).
export const USER_ROLES: UserRole[] = ["admin"];

// ID of the built-in admin permission role. The base "admin" role already
// stands for it in the user role picker.
export const ADMIN_ROLE_ID = "admin";

export const DEFAULT_USER_ROLE: UserRole = "";

// Scope at which custom roles are bound from the user directory.
export const PLATFORM_SCOPE: RbacScope = { kind: "platform" };
