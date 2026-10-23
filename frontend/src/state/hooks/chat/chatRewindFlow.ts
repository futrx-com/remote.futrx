import type { ChatStatus } from "../../../models/chat";
import type { ConfirmFn } from "../../context/ConfirmContext";

interface ChatRewindFlowOptions {
  status: ChatStatus;
  beforeT: number;
  promptText: string;
  confirm: ConfirmFn;
  rewind: (beforeT: number) => Promise<unknown>;
  clearQueuedPrompts: () => void;
  restorePrompt: (text: string) => void;
  refreshMeta: (throwOnError?: boolean) => Promise<void>;
  onComplete: () => void;
}

/** Keep all rewind feedback in the app's dialog, not browser-native alerts. */
export async function runChatRewind({
  status,
  beforeT,
  promptText,
  confirm,
  rewind,
  clearQueuedPrompts,
  restorePrompt,
  refreshMeta,
  onComplete,
}: ChatRewindFlowOptions): Promise<void> {
  if (status === "streaming") {
    await confirm({
      title: "Rewind unavailable",
      description: "An agent run is in progress.",
      message: "Cancel the current run before rewinding this chat.",
      confirmLabel: "Got it",
      cancelLabel: "Close",
      tone: "neutral",
    });
    return;
  }

  const confirmed = await confirm({
    title: "Rewind chat",
    description: "This action cannot be undone.",
    message: "Every message from this prompt forward is removed, and the prompt is put back in the composer.",
    confirmLabel: "Rewind",
    pendingLabel: "Rewinding…",
    // ConfirmDialog shows a pending indicator and keeps API errors inline for retry.
    action: () => rewind(beforeT),
  });
  if (!confirmed) return;

  // The server has completed the destructive action. Do not modify local drafts
  // or queues if the user cancelled or the rewind request failed.
  clearQueuedPrompts();
  restorePrompt(promptText);
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
