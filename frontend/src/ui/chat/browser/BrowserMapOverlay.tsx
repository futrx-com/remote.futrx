import type { FlowNode } from "../../../models/flow";

export function BrowserMapOverlay({
  activeNode,
}: {
  activeNode?: FlowNode;
}) {
  if (!activeNode || !activeNode.payload) return null;

  const { x, y, width, height, selector } = activeNode.payload;
  const hasCoords = x !== undefined && y !== undefined;
  if (!hasCoords && !selector) return null;

  // Viewport mapping default (1366x768 display)
  const posX = hasCoords ? (x / 1366) * 100 : 50;
  const posY = hasCoords ? (y / 768) * 100 : 50;

  return (
    <div class="absolute inset-0 z-30 pointer-events-none overflow-hidden">
      {/* Coordinate Click Marker Pin */}
      {hasCoords && (
        <div
          style={`left: ${posX}%; top: ${posY}%;`}
          class="absolute -translate-x-1/2 -translate-y-1/2 flex items-center justify-center"
        >
          {/* Pulsing Target Ring */}
          <div class="absolute w-12 h-12 rounded-full border-2 border-cyan-400 animate-ping opacity-75" />
          <div class="absolute w-8 h-8 rounded-full border-2 border-cyan-400 bg-cyan-500/20 shadow-[0_0_15px_rgba(6,182,212,0.8)]" />
          <div class="relative w-3 h-3 rounded-full bg-cyan-300 shadow-md" />
          {/* Label tag */}
          <div class="absolute top-6 left-1/2 -translate-x-1/2 px-2 py-0.5 rounded bg-cyan-950/90 text-cyan-300 border border-cyan-500/60 font-mono text-[10px] whitespace-nowrap shadow-xl">
            {activeNode.verb.toUpperCase()} ({x}, {y})
          </div>
        </div>
      )}

      {/* Target Bounding Box Highlight if dimensions are available */}
      {width && height && hasCoords && (
        <div
          style={`left: ${posX}%; top: ${posY}%; width: ${(width / 1366) * 100}%; height: ${(height / 768) * 100}%;`}
          class="absolute border-2 border-dashed border-cyan-400 bg-cyan-500/10 rounded backdrop-blur-[1px] shadow-[0_0_20px_rgba(6,182,212,0.4)]"
        />
      )}
    </div>
  );
}
