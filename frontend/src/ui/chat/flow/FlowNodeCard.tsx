import type { FlowNode } from "../../../models/flow";

export function FlowNodeCard({
  node,
  selected,
  onSelect,
}: {
  node: FlowNode;
  selected: boolean;
  onSelect: (nodeId: string) => void;
}) {
  const isGated = node.status === "gated" || node.type === "gate";
  const isProcessing = node.status === "processing";
  const isCompleted = node.status === "completed";
  const isFailed = node.status === "failed";

  let statusBorder = "border-line hover:border-cyan-500/50";
  let statusGlow = "";
  let badgeBg = "bg-surface-elevated text-text-muted";

  if (isProcessing) {
    statusBorder = "border-cyan-500/80 shadow-[0_0_15px_rgba(6,182,212,0.35)] animate-pulse";
    statusGlow = "bg-cyan-500/10";
    badgeBg = "bg-cyan-500/20 text-cyan-400 border border-cyan-500/40";
  } else if (isCompleted) {
    statusBorder = "border-emerald-500/60 shadow-[0_0_12px_rgba(16,185,129,0.2)]";
    statusGlow = "bg-emerald-500/5";
    badgeBg = "bg-emerald-500/20 text-emerald-400 border border-emerald-500/30";
  } else if (isFailed) {
    statusBorder = "border-rose-500/80 shadow-[0_0_15px_rgba(244,63,94,0.3)]";
    statusGlow = "bg-rose-500/10";
    badgeBg = "bg-rose-500/20 text-rose-400 border border-rose-500/40";
  } else if (isGated) {
    statusBorder = "border-amber-500/80 shadow-[0_0_18px_rgba(245,158,11,0.4)] animate-bounce";
    statusGlow = "bg-amber-500/15";
    badgeBg = "bg-amber-500/25 text-amber-300 border border-amber-500/50 font-bold";
  }

  if (selected) {
    statusBorder += " ring-2 ring-cyan-400 ring-offset-2 ring-offset-background";
  }

  const urlVal = node.payload?.url || node.payload?.path;
  const coords =
    node.payload?.x !== undefined && node.payload?.y !== undefined
      ? `(${node.payload.x}, ${node.payload.y})`
      : null;

  return (
    <div
      onClick={() => onSelect(node.id)}
      class={`relative w-72 rounded-xl border p-4 cursor-pointer transition-all duration-200 backdrop-blur-md bg-surface/90 ${statusBorder} ${statusGlow}`}
    >
      <div class="flex items-center justify-between gap-2 mb-2">
        <div class="flex items-center gap-2">
          <span class="text-xl select-none">{node.icon || "⚡"}</span>
          <span class="font-semibold text-sm text-text truncate max-w-[140px]">
            {node.title}
          </span>
        </div>
        {node.binaryPath && (
          <span
            title="Maestro Binary Address"
            class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-surface-elevated text-cyan-400 border border-cyan-500/30 select-none"
          >
            {node.binaryPath}
          </span>
        )}
      </div>

      {node.description && (
        <p class="text-xs text-text-muted mb-2 font-mono truncate select-all">
          {node.description}
        </p>
      )}

      <div class="flex flex-wrap items-center justify-between text-xs gap-1 mt-2 pt-2 border-t border-line/50">
        <span class={`px-2 py-0.5 rounded-full text-[11px] font-medium capitalize ${badgeBg}`}>
          {node.verb || node.status}
        </span>

        {coords && (
          <span class="text-[11px] font-mono text-cyan-400 bg-cyan-950/60 px-1.5 py-0.5 rounded border border-cyan-800/50">
            🎯 {coords}
          </span>
        )}
      </div>

      {urlVal && (
        <div class="mt-2 text-[11px] font-mono text-text-muted truncate bg-surface-elevated/80 p-1.5 rounded border border-line/40 select-all">
          🔗 {urlVal}
        </div>
      )}
    </div>
  );
}
