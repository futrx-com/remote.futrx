import type { PermissionBranch } from "../../../state/hooks/permissions/permissionBranches";

export function BranchOverviewCard({ branch, onOpen }: { branch: PermissionBranch; onOpen: () => void }) {
  const enabledShare = branch.total === 0 ? 0 : (branch.allowed / branch.total) * 100;
  const deniedShare = branch.total === 0 ? 0 : (branch.denied / branch.total) * 100;
  return (
    <button
      type="button"
      onClick={onOpen}
      aria-label={`Configure ${branch.id} permissions`}
      class="group flex flex-col gap-3 rounded-card border border-line bg-surface p-3.5 text-left transition-[border-color,transform,box-shadow] duration-150 hover:-translate-y-0.5 hover:border-accent-blue/50 hover:shadow-[0_8px_24px_rgba(0,0,0,.25)] focus-visible:outline focus-visible:outline-2 focus-visible:outline-accent-blue"
    >
      <div class="flex items-center justify-between gap-2">
        <span class="text-[14px] font-semibold capitalize text-ink-50">{branch.id}</span>
        {branch.customized > 0 && (
          <span class="inline-flex h-5 items-center rounded bg-accent-blue/15 px-1.5 text-[11px] font-medium text-accent-blue">
            {branch.customized} changed
          </span>
        )}
      </div>
      <div class="flex h-1.5 overflow-hidden rounded-full bg-tint-strong" aria-hidden="true">
        <div class="bg-accent-green" style={{ width: `${enabledShare}%` }} />
        <div class="bg-accent-red" style={{ width: `${deniedShare}%` }} />
      </div>
      <div class="flex items-center gap-3 text-[12px] text-ink-300">
        <span>{branch.total} permissions</span>
        <span class="text-accent-green">{branch.allowed} allowed</span>
        <span class="text-accent-red">{branch.denied} denied</span>
      </div>
      <span class="text-[11.5px] text-ink-400 transition-colors group-hover:text-accent-blue">Configure →</span>
    </button>
  );
}
