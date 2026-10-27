import { ArrowUp, Clock, Square } from "../../primitives/icons";

export function SendControls({
  streaming,
  canSend,
  disconnected,
  onCancel,
}: {
  streaming: boolean;
  canSend: boolean;
  disconnected: boolean;
  onCancel: () => void;
}) {
  return (
    <div class="codex-send-controls flex flex-none items-center gap-1.5">
      <button
        type="submit"
        disabled={!canSend}
        class="codex-send-button grid h-8 w-8 flex-none place-items-center rounded-full
               bg-accent-blue text-app transition hover:brightness-110
               active:scale-[0.97] disabled:bg-tint-active disabled:text-ink-400
               disabled:cursor-not-allowed disabled:active:scale-100"
        aria-label={streaming ? "Queue prompt" : "Send"}
        title={canSend ? (streaming ? "Queue prompt" : "Send") : disconnected ? "Connecting" : "Send"}
      >
        {streaming ? <Clock class="h-3.5 w-3.5" /> : <ArrowUp class="h-4 w-4" />}
      </button>
      {streaming && (
        <button
          type="button"
          onClick={onCancel}
          class="codex-cancel-button grid h-8 w-8 flex-none place-items-center rounded-full
                 bg-accent-red/15 text-accent-red transition hover:bg-accent-red/25 active:scale-[0.97]"
          aria-label="Cancel"
          title="Cancel current generation"
        >
          <Square class="h-3 w-3" />
        </button>
      )}
    </div>
  );
}
