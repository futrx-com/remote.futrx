import { API_ROUTES } from "../../config/routes.ts";
import type { AppBackendInstance } from "../../models/application.ts";
import type { ExtensionBackendTarget } from "../../models/extension.ts";

/**
 * Owns the invariant that every backend call resolves to one running install
 * before a URL is constructed. Selection order is part of the extension API:
 * explicit instance, matching project, global install, then first install.
 */
export class ExtensionBackendTargets {
  readonly instances: AppBackendInstance[];
  private readonly imageId: string;

  constructor(
    imageId: string,
    backends: AppBackendInstance[],
  ) {
    this.imageId = imageId;
    this.instances = [...backends];
  }

  url(path: string, target?: ExtensionBackendTarget): string {
    const instance = this.resolve(target);
    return instance.scope === "project" && instance.projectId
      ? API_ROUTES.projects.applicationBackend(
        instance.projectId,
        instance.instanceId,
        path,
      )
      : API_ROUTES.applications.backend(instance.instanceId, path);
  }

  private resolve(target?: ExtensionBackendTarget): AppBackendInstance {
    if (!this.instances.length) {
      throw new Error(`${this.imageId} has no running backend to call`);
    }
    if (target?.instanceId) {
      const chosen = this.instances.find(
        (candidate) => candidate.instanceId === target.instanceId,
      );
      if (!chosen) {
        throw new Error(
          `${this.imageId} has no running backend with id ${target.instanceId}`,
        );
      }
      return chosen;
    }

    // A project id is a preference, not a filter: an extension passing
    // `context.projectId` wants "this project's backend, or the server-wide
    // one", which is exactly how the same extension is scoped on screen.
    if (target?.projectId) {
      const inProject = this.instances.find(
        (candidate) => candidate.projectId === target.projectId,
      );
      if (inProject) return inProject;
    }
    return this.instances.find((candidate) => candidate.scope === "global")
      ?? this.instances[0];
  }
}
