import { Folder, Plus, Search } from "../primitives/icons";

export function SidebarEmptyState({ onNewProject }: { onNewProject: () => void }) {
  return (
    <div class="mx-1 rounded-card border border-dashed border-line-strong px-4 py-8 text-center text-sm text-ink-300">
      <Folder class="mx-auto mb-3 h-7 w-7 text-ink-400" />
      <div class="text-[13px] font-medium text-ink-100">No projects yet</div>
      <div class="mt-1.5 text-[12px] leading-relaxed text-ink-400">
        Each project gets its own sandboxed container. Agent CLIs run inside.
      </div>
      <button
        type="button"
        onClick={onNewProject}
        class="mt-4 inline-flex h-8 items-center gap-1.5 rounded-control bg-tint-strong px-3 text-[13px] font-medium text-ink-100 transition-colors hover:bg-tint-active"
      >
        <Plus class="w-4 h-4" /> New project
      </button>
    </div>
  );
}

export function SidebarNoMatches() {
  return (
    <div class="mx-1 rounded-card border border-dashed border-line-strong px-4 py-6 text-center text-[13px] text-ink-400">
      <Search class="mx-auto mb-2 h-5 w-5" />
      No matches
    </div>
  );
}
