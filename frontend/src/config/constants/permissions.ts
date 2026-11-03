export const PERMISSIONS_CHANGED_EVENT = "permissions-changed";
export const EFFECTIVE_PERMISSIONS_REFRESH_MS = 30000;

export const DEFAULT_PERMISSION_CATEGORY = "general";

export const PERMISSION_BASELINE_LABELS: Record<string, string> = {
  none: "no default access",
  admin: "administrators by default",
  "project-member": "project members by default",
  authenticated: "everyone by default",
};

// Form limits mirror the existing permission API contract.
export const ROLE_NAME_MAX = 80;
export const ROLE_DESCRIPTION_MAX = 500;

export const PERMISSIONS_EMPTY_COPY = {
  roles: "No roles yet.",
  assignments: "No assignments yet.",
  bindings: "No role bindings yet.",
  definitions: "No permissions registered.",
} as const;
