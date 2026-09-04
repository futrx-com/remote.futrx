import type { ChatStatus, PromptOutcome, QueuedPrompt } from "../../../models/chat";

// Delivery policy for queued prompts. A queued prompt is only removed once the
// server acknowledges that a run accepted it; a rejected or unacknowledged
// dispatch keeps the prompt queued so it retries in the next send window
// instead of being silently lost. The dispatch latch guarantees at most one
// prompt is on the wire per window.
class PromptQueueState {
  // The prompt to put on the wire now, or null if the window is closed, a
  // dispatch is already in flight, or the queue is empty.
  nextDispatch(
    prompts: QueuedPrompt[],
    inflightId: string | null,
    status: ChatStatus,
    canSendPrompt: boolean,
  ): QueuedPrompt | null {
    if (status !== "ready" || !canSendPrompt) return null;
    if (inflightId !== null) return null;
    return prompts[0] ?? null;
  }

  // Prompts remaining after the server's verdict: accepted removes the prompt
  // (it now lives in the transcript), rejected keeps it for the next window.
  promptsAfterOutcome(prompts: QueuedPrompt[], outcome: PromptOutcome): QueuedPrompt[] {
    if (!outcome.accepted) return prompts;
    if (!prompts.some((prompt) => prompt.id === outcome.clientId)) return prompts;
    return prompts.filter((prompt) => prompt.id !== outcome.clientId);
  }

  // Whether a prompt typed now should be queued rather than sent: a run is
  // already streaming, so the composer parks it for the next send window.
  allowsQueue(status: ChatStatus): boolean {
    return status === "streaming";
  }

  // The latch after an outcome: any verdict for the in-flight prompt frees it.
  inflightAfterOutcome(inflightId: string | null, outcome: PromptOutcome): string | null {
    return inflightId === outcome.clientId ? null : inflightId;
  }
}

export const promptQueueState = new PromptQueueState();
