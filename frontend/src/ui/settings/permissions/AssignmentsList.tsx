import type { ProjectMeta } from "../../../models/project";
import type { RbacAssignment, RbacAssignmentInput, RbacAssignmentTarget, RbacDefinition } from "../../../models/rbac";
import { useConfirm } from "../../../state/context/ConfirmContext";
import { X } from "../../primitives/icons";
import { AddAssignmentForm } from "./AddAssignmentForm";
import { PERMISSIONS_EMPTY_COPY } from "../../../state/hooks/permissions/permissionsView";
import { Empty } from "../../projects/project-containers/ProjectContainerPrimitives";
import { EffectBadge, PermissionsRow, PermissionsSection, ScopeBadge } from "./PermissionsPrimitives";

export function AssignmentsList({
  assignments,
  definitions,
  projects,
  loading,
  onAdd,
  onRemove,
}: {
  assignments: RbacAssignment[];
  definitions: RbacDefinition[];
  projects: ProjectMeta[];
  loading: boolean;
  onAdd: (input: RbacAssignmentInput) => Promise<void>;
  onRemove: (target: RbacAssignmentTarget) => Promise<void>;
}) {
  const confirm = useConfirm();

  const remove = async (assignment: RbacAssignment) => {
    await confirm({
      title: "Remove assignment",
      description: "The user loses this permission setting immediately.",
      message: `${assignment.userEmail} — ${assignment.effect} ${assignment.permission} will be removed.`,
      confirmLabel: "Remove",
      pendingLabel: "Removing…",
      tone: "danger",
      action: () =>
        onRemove({
          userEmail: assignment.userEmail,
          permission: assignment.permission,
          scope: assignment.scope,
        }),
    });
  };

  return (
    <PermissionsSection
      title="Assignments"
      description="Permissions granted or denied directly to a user."
      loading={loading}
    >
      <AddAssignmentForm definitions={definitions} projects={projects} onAdd={onAdd} />
      {assignments.length === 0 ? (
        <Empty text={PERMISSIONS_EMPTY_COPY.assignments} compact />
      ) : (
        assignments.map((assignment) => (
          <PermissionsRow key={assignment.id}>
            <span class="text-[12.5px] text-ink-50 truncate" title={assignment.userEmail}>
              {assignment.userEmail}
            </span>
            <span class="font-mono text-[12px] text-ink-200 truncate" title={assignment.permission}>
              {assignment.permission}
            </span>
            <EffectBadge effect={assignment.effect} />
            <ScopeBadge scope={assignment.scope} />
            <div class="ml-auto flex items-center gap-1">
              <button
                type="button"
                onClick={() => void remove(assignment)}
                class="h-7 w-7 rounded text-ink-300 hover:text-accent-red hover:bg-tint-strong grid place-items-center disabled:opacity-50"
                aria-label="Remove assignment"
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
