import type { ProjectMeta } from "../../models/project";
import type { PermissionsController } from "../../state/hooks/permissions/usePermissions";
import { permissionsViewState } from "../../state/hooks/permissions/permissionsView";
import { AlertCircle, Loader } from "../primitives/icons";
import { AssignmentsList } from "./permissions/AssignmentsList";
import { BindingsList } from "./permissions/BindingsList";
import { DefinitionsList } from "./permissions/DefinitionsList";
import { RolesList } from "./permissions/RolesList";

export function PermissionsSettings({
  permissions,
  projects,
}: {
  permissions: PermissionsController;
  projects: ProjectMeta[];
}) {
  const { loading, loaded, error, definitions, roles, assignments, bindings } = permissions;
  const view = permissionsViewState({ loading, error, loaded });

  if (view === "loading") {
    return (
      <div class="flex items-center gap-2 text-[13px] text-ink-300">
        <Loader class="w-4 h-4 animate-spin" /> Loading permissions…
      </div>
    );
  }

  if (view === "error") {
    return (
      <div class="flex items-start gap-2.5 rounded-lg border border-accent-red/30 bg-accent-red/[0.08] px-3 py-2.5 text-[13px]">
        <AlertCircle class="w-4 h-4 mt-0.5 flex-none text-accent-red" />
        <div class="text-accent-red break-words">{error}</div>
      </div>
    );
  }

  return (
    <div class="space-y-4">
      <RolesList
        roles={roles}
        bindings={bindings}
        definitions={definitions}
        loading={loading}
        onCreate={permissions.createRole}
        onUpdate={permissions.updateRole}
        onDelete={permissions.deleteRole}
      />
      <AssignmentsList
        assignments={assignments}
        definitions={definitions}
        projects={projects}
        loading={loading}
        onAdd={permissions.addAssignment}
        onRemove={permissions.removeAssignment}
      />
      <BindingsList
        bindings={bindings}
        roles={roles}
        definitions={definitions}
        projects={projects}
        loading={loading}
        onAdd={permissions.addBinding}
        onRemove={permissions.removeBinding}
      />
      <DefinitionsList definitions={definitions} />
    </div>
  );
}
