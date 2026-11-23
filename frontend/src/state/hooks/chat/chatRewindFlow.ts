import type { ChatStatus } from "../../../models/chat";
import type { ConfirmFn } from "../../context/ConfirmContext";

interface ChatRewindFlowOptions {
  status: ChatStatus;
  rewindBeforeTimestamp: number;
  promptText: string;
  confirm: ConfirmFn;
  stopAgent: () => Promise<void>;
  rewind: (rewindBeforeTimestamp: number) => Promise<unknown>;
  clearQueuedPrompts: () => void;
  restorePrompt: (text: string) => void;
  /**
   * Retry instead of edit: the prompt is sent again once the chat is ready,
   * rather than put back in the composer.
   */
  resend?: (text: string) => void;
  refreshMeta: (throwOnError?: boolean) => Promise<void>;
  onComplete: () => void;
}

/** Keep all rewind feedback in the app's dialog, not browser-native alerts. */
export async function runChatRewind({
  status,
  rewindBeforeTimestamp,
  promptText,
  confirm,
  stopAgent,
  rewind,
  clearQueuedPrompts,
  restorePrompt,
  resend,
  refreshMeta,
  onComplete,
}: ChatRewindFlowOptions): Promise<void> {
  const hasActiveRun = status === "streaming";

  const confirmed = await confirm({
    title: resend ? "Retry response" : "Rewind chat",
    description: "This action cannot be undone.",
    message: hasActiveRun
      ? "The current agent run will be stopped, then every message from this prompt forward will be removed. " +
        (resend ? "The prompt will be sent again." : "The prompt will be restored to the composer.")
      : "Every message from this prompt forward is removed, and the prompt is " +
        (resend ? "sent again." : "put back in the composer."),
    confirmLabel: resend
      ? (hasActiveRun ? "Stop and retry" : "Retry")
      : (hasActiveRun ? "Stop and rewind" : "Rewind"),
    pendingLabel: hasActiveRun ? "Stopping and rewinding…" : "Rewinding…",
    // Cancel only requests a stop. Wait for the backend run lock to clear
    // before rewinding; failures remain inline and retryable in ConfirmDialog.
    action: async () => {
      if (hasActiveRun) await stopAgent();
      await rewind(rewindBeforeTimestamp);
    },
  });
  if (!confirmed) return;

  // The server has completed the destructive action. Do not modify local drafts
  // or queues if the user cancelled or the rewind request failed.
  clearQueuedPrompts();
  if (resend) resend(promptText);
  else restorePrompt(promptText);
  try {
    await refreshMeta(true);
  } catch (error) {
    const message = error instanceof Error && error.message.trim()
      ? error.message
      : "Reload the chat to see the latest state.";
    try {
      await confirm({
        title: "Chat refresh failed",
        description: "The rewind completed, but the chat could not refresh.",
        message,
        confirmLabel: "Got it",
        cancelLabel: "Close",
        tone: "neutral",
      });
    } finally {
      // The rewind must never be retried just because the metadata refresh failed.
      onComplete();
    }
    return;
  }
  onComplete();
}
