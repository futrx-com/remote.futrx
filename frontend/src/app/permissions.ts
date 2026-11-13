import { PermissionLookupService } from "../services/permissions/permissionLookupService";
import { effectivePermissionsApi, permissionsApi } from "../api/permissions";
import { PERMISSIONS_CHANGED_EVENT } from "../config/constants/permissions";
import { PermissionAdministrationService } from "../services/permissions/permissionAdministrationService";
import { createPermissionAdministrationStore } from "../state/stores/permissions/permissionAdministrationStore";

/** Constructed once per SettingsContainer mount, preserving the original hook lifetime. */
export function createPermissionAdministration() {
  const store = createPermissionAdministrationStore();
  const commands = new PermissionAdministrationService(
    permissionsApi,
    store,
    () => window.dispatchEvent(new Event(PERMISSIONS_CHANGED_EVENT)),
  );
  return { store, commands };
}

export const permissionLookup = new PermissionLookupService({
  effective: effectivePermissionsApi.effective,
  listAccountTargets: permissionsApi.listAccountTargets,
});
