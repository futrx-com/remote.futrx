const FIRST_RETRY_MS = 1000;
const SLOWEST_RETRY_MS = 10000;

/**
 * What to do once the shell's output stream has stopped. `ended` means the
 * shell itself exited: reconnecting then would start a new shell each time
 * one exits, so the pane waits to be asked.
 */
export function afterFailure({ ended, visible }) {
  if (ended) return "ended";
  // A closed pane does not keep retrying; it reconnects when shown.
  return visible ? "retry" : "wait";
}

/** Delay before retry number `attempt`, counting from zero. */
export function retryDelayMs(attempt) {
  return Math.min(FIRST_RETRY_MS * 2 ** Math.max(0, attempt), SLOWEST_RETRY_MS);
}
