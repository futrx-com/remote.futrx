// JumpToLatestButton — floating square button pinned to the thread's bottom-right corner.
// It positions itself absolutely, so the preview supplies a relative thread-like stage.
import { JumpToLatestButton } from "remote.futrx-web";

export const OverThread = () => (
  <div
    className="relative w-full max-w-xl overflow-hidden rounded-lg border border-line"
    style={{ height: "11rem" }}
  >
    <div className="p-4 space-y-3 text-sm text-ink-300">
      <div>…earlier messages scrolled out of view…</div>
      <div className="text-ink-100">
        The migration ran cleanly on staging — 412 rows backfilled, no orphaned sessions.
      </div>
      <div>Running the same script against production next.</div>
    </div>
    <JumpToLatestButton onClick={() => {}} />
  </div>
);
