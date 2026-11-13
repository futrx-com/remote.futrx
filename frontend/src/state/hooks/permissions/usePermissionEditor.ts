import { permissionErrors } from "../../../api/permissionErrors";
import { permissionBranchService } from "../../../services/permissions/permissionBranchService";
import { permissionFormService } from "../../../services/permissions/permissionFormService";

/** UI gateway to permission validation, role rules, and server error mapping. */
export function usePermissionEditor() {
  return {
    forms: permissionFormService,
    branches: permissionBranchService,
    errors: permissionErrors,
  };
}
