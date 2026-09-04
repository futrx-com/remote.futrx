import type { ProcessTreeNode } from "../../../models/flow";

export function ProcessTreeMap({ tree }: { tree: ProcessTreeNode[] }) {
  if (!tree || tree.length === 0) return null;

  return (
    <div class="bg-surface/80 border border-line rounded-xl p-3 backdrop-blur-md text-xs">
      <div class="flex items-center gap-2 font-semibold text-text mb-2 pb-1.5 border-b border-line/60">
        <span class="text-cyan-400 select-none">🗺️</span>
        <span>Maestro Process Tree</span>
      </div>
      <div class="space-y-1.5 font-mono">
        {tree.map((item) => (
          <div key={item.id} class="flex items-center justify-between gap-2 p-1.5 rounded bg-surface-elevated/60 hover:bg-surface-elevated transition-colors">
            <div class="flex items-center gap-2 truncate">
              <span class="text-[10px] text-cyan-400 bg-cyan-950 px-1 py-0.5 rounded border border-cyan-800">
                {item.binaryPath}
              </span>
              <span class="truncate text-text">{item.title}</span>
            </div>
            <span class="text-[10px] text-text-muted capitalize">
              {item.status}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}
