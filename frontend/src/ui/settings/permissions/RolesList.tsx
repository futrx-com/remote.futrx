import { useState } from "preact/hooks";
import type { RbacBinding, RbacDefinition, RbacRole, RbacRoleInput } from "../../../models/rbac";
import { PERMISSIONS_EMPTY_COPY } from "../../../state/hooks/permissions/permissionsView";
import { Empty } from "../../projects/project-containers/ProjectContainerPrimitives";
import { X } from "../../primitives/icons";
import { DeleteRoleModal } from "./DeleteRoleModal";
import { PermissionBadge, PermissionsRow, PermissionsSection } from "./PermissionsPrimitives";
import { RoleFormModal } from "./RoleFormModal";

type RoleModal = { kind: "create" } | { kind: "edit"; role: RbacRole } | { kind: "delete"; role: RbacRole };

export function RolesList({
  roles,
  bindings,
  definitions,
  loading,
  onCreate,
  onUpdate,
  onDelete,
}: {
  roles: RbacRole[];
  bindings: RbacBinding[];
  definitions: RbacDefinition[];
  loading: boolean;
  onCreate: (input: RbacRoleInput) => Promise<void>;
  onUpdate: (id: string, input: RbacRoleInput) => Promise<void>;
  onDelete: (id: string, unbind: boolean) => Promise<void>;
}) {
  const [modal, setModal] = useState<RoleModal | null>(null);
  const close = () => setModal(null);

  return (
    <PermissionsSection
      title="Roles"
      description="Named sets of allow and deny rules that can be bound to users."
      loading={loading}
    >
      <div class="flex justify-end">
        <button
          type="button"
          onClick={() => setModal({ kind: "create" })}
          class="btn btn-primary btn-sm"
        >
          Create role
        </button>
      </div>
      {roles.length === 0 ? (
        <Empty text={PERMISSIONS_EMPTY_COPY.roles} compact />
      ) : (
        roles.map((role) => (
          <PermissionsRow key={role.id}>
            <span class="text-[12.5px] text-ink-50 truncate" title={role.description || role.name}>
              {role.name}
            </span>
            <PermissionBadge tone="neutral">
              {`${role.rules.length} rule${role.rules.length === 1 ? "" : "s"}`}
            </PermissionBadge>
            <div class="ml-auto flex items-center gap-1">
              <button
                type="button"
                onClick={() => setModal({ kind: "edit", role })}
                class="h-7 px-2 rounded text-[11px] text-ink-300 hover:text-ink-100 hover:bg-tint-strong disabled:opacity-50"
              >
                edit
              </button>
              <button
                type="button"
                onClick={() => setModal({ kind: "delete", role })}
                class="h-7 w-7 rounded text-ink-300 hover:text-accent-red hover:bg-tint-strong grid place-items-center disabled:opacity-50"
                aria-label={`Delete ${role.name}`}
                title="Delete"
              >
                <X class="w-3.5 h-3.5" />
              </button>
            </div>
          </PermissionsRow>
        ))
      )}
      {modal?.kind === "create" && (
        <RoleFormModal roles={roles} definitions={definitions} onSubmit={onCreate} onClose={close} />
      )}
      {modal?.kind === "edit" && (
        <RoleFormModal
          role={modal.role}
          roles={roles}
          definitions={definitions}
          onSubmit={(input) => onUpdate(modal.role.id, input)}
          onClose={close}
        />
      )}
      {modal?.kind === "delete" && (
        <DeleteRoleModal
          role={modal.role}
          bindingCount={bindings.filter((binding) => binding.roleId === modal.role.id).length}
          onDelete={(unbind) => onDelete(modal.role.id, unbind)}
          onClose={close}
        />
      )}
    </PermissionsSection>
  );
}
