export type PermissionsViewState = "loading" | "error" | "ready";

export const PERMISSIONS_EMPTY_COPY = {
  roles: "No roles yet.",
  assignments: "No assignments yet.",
  bindings: "No role bindings yet.",
  definitions: "No permissions registered.",
} as const;

/**
 * Which screen the Permissions tab shows. A failed load renders the error
 * alone: the lists would otherwise claim "No roles yet." about data that was
 * never fetched.
 */
export function permissionsViewState(input: {
  loading: boolean;
  error: string | null;
  loaded: boolean;
}): PermissionsViewState {
  if (input.error) return "error";
  if (input.loading && !input.loaded) return "loading";
  return "ready";
}
