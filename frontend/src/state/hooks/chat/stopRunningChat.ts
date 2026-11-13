interface StopRunningChatOptions {
  isRunning: () => Promise<boolean>;
  sendCancel: () => boolean;
  wait?: (milliseconds: number) => Promise<void>;
  now?: () => number;
  timeoutMs?: number;
  pollIntervalMs?: number;
}

/** Cancellation is asynchronous; wait for the server's run lock to be released. */
export async function stopRunningChat({
  isRunning,
  sendCancel,
  wait = (milliseconds) => new Promise<void>((resolve) => setTimeout(resolve, milliseconds)),
  now = Date.now,
  timeoutMs = 20_000,
  pollIntervalMs = 250,
}: StopRunningChatOptions): Promise<void> {
  // The UI may still show streaming after the run ended. Avoid a redundant
  // cancel message that would be rejected by the WebSocket handler.
  if (!(await isRunning())) return;
  if (!sendCancel()) {
    throw new Error("Unable to stop the agent. Reconnect to the chat and try again.");
  }

  const deadline = now() + timeoutMs;
  while (true) {
    if (!(await isRunning())) return;
    const remainingMs = deadline - now();
    if (remainingMs <= 0) {
      throw new Error("The agent is still stopping. Rewind was not attempted; please try again.");
    }
    await wait(Math.min(pollIntervalMs, remainingMs));
  }
}
