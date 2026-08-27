// ComposerDropOverlay — dashed drop target shown above the composer while dragging files.
// The component positions itself absolutely (inset-x-3 -top-16), so each story
// anchors it to an inner relative box pushed down inside a sized outer card.
import { ComposerDropOverlay } from "remote.futrx-web";

export const OverComposer = () => (
  <div className="w-full max-w-xl" style={{ height: 190, paddingTop: 84 }}>
    <div className="relative">
      <ComposerDropOverlay />
      {/* Mock composer footprint for context */}
      <div className="rounded-lg border border-white/10 bg-[#101318] px-3 py-3">
        <div className="text-sm text-ink-500">Message the agent...</div>
        <div className="flex items-center gap-1.5" style={{ marginTop: 28 }}>
          <div className="w-10 h-10 rounded-lg bg-white/[0.045]" />
          <div style={{ flex: 1 }} />
          <div className="w-10 h-10 rounded-lg bg-accent-green" />
        </div>
      </div>
    </div>
  </div>
);

export const Bare = () => (
  <div className="w-full max-w-xl" style={{ height: 110, paddingTop: 84 }}>
    <div className="relative">
      <ComposerDropOverlay />
    </div>
  </div>
);
