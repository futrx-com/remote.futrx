import type {
  RbacAssignmentInput, RbacAssignmentTarget,
  RbacBindingInput, RbacBindingTarget, RbacRoleInput,
} from "../../models/rbac.ts";
import type {
  PermissionAdministrationApi, PermissionAdministrationCommands, PermissionAdministrationStore,
} from "../../port/permissions.ts";

/** Owns mutation → notification → reload ordering for one administration view. */
export class PermissionAdministrationService implements PermissionAdministrationCommands {
  private readonly api: PermissionAdministrationApi;
  private readonly store: PermissionAdministrationStore;
  private readonly notifyChanged: () => void;

  constructor(
    api: PermissionAdministrationApi,
    store: PermissionAdministrationStore,
    notifyChanged: () => void,
  ) {
    this.api = api;
    this.store = store;
    this.notifyChanged = notifyChanged;
  }

  refresh = async (): Promise<void> => {
    this.store.getState().beginRefresh();
    try {
      const [definitions, roles, assignments, bindings] = await Promise.all([
        this.api.listDefinitions(),
        this.api.listRoles(),
        this.api.listAssignments(),
        this.api.listBindings(),
      ]);
      this.store.getState().receiveSnapshot({ definitions, roles, assignments, bindings });
    } catch (cause) {
      this.store.getState().failRefresh((cause as Error).message);
    } finally {
      this.store.getState().finishRefresh();
    }
  };

  addAssignment = (input: RbacAssignmentInput): Promise<void> =>
    this.change(() => this.api.addAssignment(input));

  removeAssignment = (target: RbacAssignmentTarget): Promise<void> =>
    this.change(() => this.api.removeAssignment(target));

  addBinding = (input: RbacBindingInput): Promise<void> =>
    this.change(() => this.api.addBinding(input));

  removeBinding = (target: RbacBindingTarget): Promise<void> =>
    this.change(() => this.api.removeBinding(target));

  createRole = (input: RbacRoleInput): Promise<void> =>
    this.change(() => this.api.createRole(input));

  updateRole = (id: string, input: RbacRoleInput): Promise<void> =>
    this.change(() => this.api.updateRole(id, input));

  deleteRole = (id: string, unbind: boolean): Promise<void> =>
    this.change(() => this.api.deleteRole(id, unbind));

  private async change(mutate: () => Promise<unknown>): Promise<void> {
    await mutate();
    this.notifyChanged();
    await this.refresh();
  }
}
