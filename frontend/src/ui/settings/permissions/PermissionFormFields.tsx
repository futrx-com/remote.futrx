import type { ProjectMeta } from "../../../models/project";
import type { RbacScopeKind } from "../../../models/rbac";

const SELECT_CLASS =
  "h-9 px-2 rounded border border-line bg-inset text-[13px] text-ink-50 focus:outline-none focus:border-accent-blue/50";

export const INPUT_CLASS =
  "h-9 px-2.5 rounded border border-line bg-inset text-[13px] text-ink-50 placeholder-ink-400 focus:outline-none focus:border-accent-blue/50";

export function ScopeFields({
  offered,
  kind,
  projectId,
  projects,
  onKindChange,
  onProjectChange,
}: {
  offered: RbacScopeKind[];
  kind: RbacScopeKind;
  projectId: string;
  projects: ProjectMeta[];
  onKindChange: (kind: RbacScopeKind) => void;
  onProjectChange: (projectId: string) => void;
}) {
  return (
    <>
      <select
        value={kind}
        onChange={(e) => onKindChange((e.target as HTMLSelectElement).value as RbacScopeKind)}
        disabled={offered.length === 0}
        class={SELECT_CLASS}
        aria-label="Scope"
      >
        {offered.map((offeredKind) => (
          <option key={offeredKind} value={offeredKind}>{offeredKind}</option>
        ))}
      </select>
      {kind === "project" && (
        <select
          value={projectId}
          onChange={(e) => onProjectChange((e.target as HTMLSelectElement).value)}
          class={SELECT_CLASS}
          aria-label="Project"
        >
          <option value="">Choose project…</option>
          {projects.map((project) => (
            <option key={project.id} value={project.id}>{project.name}</option>
          ))}
        </select>
      )}
    </>
  );
}

export { SELECT_CLASS };
