import { useState } from "preact/hooks";
import type { FlowMapState } from "../../../models/flow";
import { FlowNodeCard } from "./FlowNodeCard";
import { ProcessTreeMap } from "./ProcessTreeMap";
import { ConfirmationGateModal } from "./ConfirmationGateModal";

export function FlowMapCanvas({
  flowState,
  selectedNodeId,
  onSelectNode,
  loading,
}: {
  flowState: FlowMapState | null;
  selectedNodeId: string | null;
  onSelectNode: (nodeId: string) => void;
  loading?: boolean;
}) {
  const [zoom, setZoom] = useState<number>(1);
  const [showTree, setShowTree] = useState<boolean>(true);
  const [activeGateNodeId, setActiveGateNodeId] = useState<string | null>(null);

  const handleZoomIn = () => setZoom((z) => Math.min(z + 0.15, 1.8));
  const handleZoomOut = () => setZoom((z) => Math.max(z - 0.15, 0.5));
  const handleResetZoom = () => setZoom(1);

  const nodes = flowState?.nodes || [];
  const checkpoints = flowState?.checkpoints || [];
  const gatedNode = nodes.find((n) => n.id === activeGateNodeId || n.status === "gated" || n.type === "gate");

  return (
    <div class="relative w-full h-full min-h-[500px] overflow-hidden bg-background select-none flex flex-col">
      {/* Top Map Toolbar */}
      <div class="relative z-20 flex items-center justify-between px-4 py-2.5 bg-surface/90 border-b border-line backdrop-blur-md">
        <div class="flex items-center gap-3">
          <div class="flex items-center gap-2">
            <span class="text-xl">🗺️</span>
            <h2 class="font-bold text-sm text-text">Computer Use Map (المسير)</h2>
          </div>
          {flowState?.activeCheckpoint && (
            <span class="text-xs px-2 py-0.5 rounded-full bg-cyan-950 text-cyan-400 border border-cyan-800 font-mono">
              Phase: {flowState.activeCheckpoint}
            </span>
          )}
          {loading && (
            <span class="text-xs text-text-muted animate-pulse">Syncing flow...</span>
          )}
        </div>

        <div class="flex items-center gap-2">
          <button
            onClick={() => setShowTree((v) => !v)}
            class={`px-2.5 py-1 text-xs rounded border transition-colors ${
              showTree
                ? "bg-cyan-500/20 text-cyan-400 border-cyan-500/40"
                : "bg-surface-elevated text-text-muted border-line hover:text-text"
            }`}
          >
            {showTree ? "Hide Maestro Tree" : "Show Maestro Tree"}
          </button>
          <div class="flex items-center rounded border border-line bg-surface-elevated overflow-hidden text-xs">
            <button
              onClick={handleZoomOut}
              class="px-2 py-1 hover:bg-surface text-text font-bold"
              title="Zoom Out"
            >
              -
            </button>
            <span class="px-2 font-mono text-text-muted">{Math.round(zoom * 100)}%</span>
            <button
              onClick={handleZoomIn}
              class="px-2 py-1 hover:bg-surface text-text font-bold"
              title="Zoom In"
            >
              +
            </button>
            <button
              onClick={handleResetZoom}
              class="px-2 py-1 border-l border-line hover:bg-surface text-text-muted hover:text-text text-[11px]"
              title="Reset Zoom"
            >
              Reset
            </button>
          </div>
        </div>
      </div>

      {/* Main Canvas Area */}
      <div class="relative flex-1 w-full h-full overflow-auto bg-grid-pattern p-8">
        {/* Visual Map Content Container */}
        <div
          style={`transform: scale(${zoom}); transform-origin: top left; transition: transform 0.15s ease-out;`}
          class="inline-flex flex-col gap-8 min-w-max pb-20"
        >
          {nodes.length === 0 ? (
            <div class="flex flex-col items-center justify-center p-12 text-center text-text-muted border border-dashed border-line rounded-2xl bg-surface/40 my-8">
              <span class="text-4xl mb-2">🧭</span>
              <p class="font-semibold text-sm text-text">No Computer Use Actions Yet</p>
              <p class="text-xs max-w-sm mt-1">
                Initiate a browser or computer task in chat (e.g. "navigate to website and search") to see live execution flow graphs.
              </p>
            </div>
          ) : (
            <div class="flex flex-wrap items-start gap-6">
              {checkpoints.length > 0
                ? checkpoints.map((cp) => {
                    const cpNodes = nodes.filter((n) => cp.nodeIds.includes(n.id));
                    return (
                      <div
                        key={cp.id}
                        class="rounded-2xl border border-cyan-500/30 bg-surface/40 p-4 backdrop-blur-sm flex flex-col gap-4 shadow-lg"
                      >
                        <div class="flex items-center justify-between gap-3 border-b border-cyan-500/20 pb-2">
                          <span class="text-xs font-bold text-cyan-400 font-mono">
                            ⚡ Checkpoint: {cp.name}
                          </span>
                          <span class="text-[10px] text-text-muted capitalize">
                            {cp.status}
                          </span>
                        </div>
                        <div class="flex flex-col gap-4">
                          {cpNodes.map((node) => (
                            <FlowNodeCard
                              key={node.id}
                              node={node}
                              selected={selectedNodeId === node.id}
                              onSelect={onSelectNode}
                            />
                          ))}
                        </div>
                      </div>
                    );
                  })
                : nodes.map((node) => (
                    <FlowNodeCard
                      key={node.id}
                      node={node}
                      selected={selectedNodeId === node.id}
                      onSelect={onSelectNode}
                    />
                  ))}
            </div>
          )}
        </div>

        {/* Floating Maestro Process Tree Overlay */}
        {showTree && flowState?.processTree && flowState.processTree.length > 0 && (
          <div class="absolute bottom-4 right-4 z-20 max-w-xs w-full shadow-2xl animate-fade-in">
            <ProcessTreeMap tree={flowState.processTree} />
          </div>
        )}
      </div>

      {/* Confirmation Gate Modal */}
      {gatedNode && (
        <ConfirmationGateModal
          node={gatedNode}
          onApprove={() => setActiveGateNodeId(null)}
          onReject={() => setActiveGateNodeId(null)}
        />
      )}
    </div>
  );
}
