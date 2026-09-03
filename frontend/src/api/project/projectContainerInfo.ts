import type {
  AuthBundleFileStatus,
  AuthBundleStatus,
  ProjectContainerInfo,
} from "../../models/project.ts";

type AuthBundlePayload = Omit<AuthBundleStatus, "files"> & {
  files?: AuthBundleFileStatus[] | null;
};

export type ProjectContainerInfoPayload = Omit<ProjectContainerInfo, "authBundles"> & {
  authBundles?: AuthBundlePayload[] | null;
};

/**
 * Keeps nullable JSON collection fields at the transport boundary. Older
 * servers encoded a configured credential directory with no files as null,
 * while the view model intentionally exposes arrays only.
 */
export function normalizeProjectContainerInfo(
  payload: ProjectContainerInfoPayload,
): ProjectContainerInfo {
  return {
    ...payload,
    authBundles: (payload.authBundles ?? []).map((bundle) => ({
      ...bundle,
      files: bundle.files ?? [],
    })),
  };
}
