import type { EffectivePermissionSnapshot, RbacAccountTarget } from "../../models/rbac.ts";
import type { PermissionLookup, PermissionLookupApi } from "../../port/permissions.ts";

/** Permission hints fail closed; an unavailable account catalog has no suggestions. */
export class PermissionLookupService implements PermissionLookup {
  private readonly api: PermissionLookupApi;

  constructor(api: PermissionLookupApi) {
    this.api = api;
  }

  async effectiveSnapshot(projectId?: string): Promise<EffectivePermissionSnapshot> {
    try {
      const values = await this.api.effective(projectId);
      return { scope: projectId, values };
    } catch {
      return { scope: projectId, values: {} };
    }
  }

  async accountTargets(): Promise<RbacAccountTarget[]> {
    try {
      return await this.api.listAccountTargets();
    } catch {
      return [];
    }
  }
}
