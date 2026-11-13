import { ApiError } from "./apiError.ts";

/** Interpret the server's role-conflict response at the API boundary. */
class PermissionErrors {
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

export const permissionErrors = new PermissionErrors();
