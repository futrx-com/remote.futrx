import type { FlowNode } from "../../../models/flow";

export function ConfirmationGateModal({
  node,
  onApprove,
  onReject,
}: {
  node: FlowNode;
  onApprove: () => void;
  onReject: () => void;
}) {
  if (node.status !== "gated" && node.type !== "gate") return null;

  const question = node.payload?.question || node.description || "Human confirmation required before executing write/sensitive action.";

  return (
    <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm animate-fade-in">
      <div class="relative w-full max-w-lg rounded-2xl border border-amber-500/50 bg-surface/95 p-6 shadow-[0_0_40px_rgba(245,158,11,0.25)] text-text">
        <div class="flex items-center gap-3 mb-4 text-amber-400">
          <span class="text-3xl select-none">🛑</span>
          <div>
            <h3 class="text-lg font-bold">Confirmation Gate (confirm)</h3>
            <p class="text-xs text-text-muted">High-stakes computer use action detected</p>
          </div>
        </div>

        <div class="my-4 rounded-xl border border-line bg-surface-elevated/80 p-4 font-mono text-sm">
          <p class="text-text select-all">{question}</p>
        </div>

        <div class="flex items-center justify-end gap-3 mt-6">
          <button
            onClick={onReject}
            class="px-4 py-2 rounded-lg border border-line hover:border-rose-500/50 hover:bg-rose-500/10 text-rose-400 text-sm font-medium transition-colors"
          >
            Reject & Abort
          </button>
          <button
            onClick={onApprove}
            class="px-5 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white text-sm font-semibold shadow-lg shadow-emerald-600/30 transition-all hover:scale-105"
          >
            Approve & Continue ➔
          </button>
        </div>
      </div>
    </div>
  );
}
