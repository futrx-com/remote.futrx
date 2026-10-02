import type { ProjectMeta } from "../../../models/project";
import type { RbacBinding, RbacBindingInput, RbacBindingTarget, RbacDefinition, RbacRole } from "../../../models/rbac";
import { useConfirm } from "../../../state/context/ConfirmContext";
import { X } from "../../primitives/icons";
import { AddBindingForm } from "./AddBindingForm";
import { PERMISSIONS_EMPTY_COPY } from "../../../state/hooks/permissions/permissionsView";
import { Empty } from "../../projects/project-containers/ProjectContainerPrimitives";
import { PermissionBadge, PermissionsRow, PermissionsSection, ScopeBadge } from "./PermissionsPrimitives";

export function BindingsList({
  bindings,
  roles,
  definitions,
  projects,
  loading,
  onAdd,
  onRemove,
}: {
  bindings: RbacBinding[];
  roles: RbacRole[];
  definitions: RbacDefinition[];
  projects: ProjectMeta[];
  loading: boolean;
  onAdd: (input: RbacBindingInput) => Promise<void>;
  onRemove: (target: RbacBindingTarget) => Promise<void>;
}) {
  const confirm = useConfirm();
  const roleName = (id: string) => roles.find((role) => role.id === id)?.name ?? id;

  const remove = async (binding: RbacBinding) => {
    await confirm({
      title: "Unbind role",
      description: "The user loses what this role grants immediately.",
      message: `${binding.userEmail} — ${roleName(binding.roleId)} will be unbound.`,
      confirmLabel: "Unbind",
      pendingLabel: "Unbinding…",
      tone: "danger",
      action: () =>
        onRemove({ roleId: binding.roleId, userEmail: binding.userEmail, scope: binding.scope }),
    });
  };

  return (
    <PermissionsSection
      title="Role bindings"
      description="Roles bound to a user at a platform or project scope."
      loading={loading}
    >
      <AddBindingForm roles={roles} definitions={definitions} projects={projects} onAdd={onAdd} />
      {bindings.length === 0 ? (
        <Empty text={PERMISSIONS_EMPTY_COPY.bindings} compact />
      ) : (
        bindings.map((binding) => (
          <PermissionsRow key={binding.id}>
            <span class="text-[12.5px] text-ink-50 truncate" title={binding.userEmail}>
              {binding.userEmail}
            </span>
            <PermissionBadge tone="highlight">
              {roleName(binding.roleId)}
            </PermissionBadge>
            <ScopeBadge scope={binding.scope} />
            <div class="ml-auto flex items-center gap-1">
              <button
                type="button"
                onClick={() => void remove(binding)}
                class="h-7 w-7 rounded text-ink-300 hover:text-accent-red hover:bg-tint-strong grid place-items-center disabled:opacity-50"
                aria-label="Unbind role"
                title="Remove"
              >
                <X class="w-3.5 h-3.5" />
              </button>
            </div>
          </PermissionsRow>
        ))
      )}
    </PermissionsSection>
  );
}
