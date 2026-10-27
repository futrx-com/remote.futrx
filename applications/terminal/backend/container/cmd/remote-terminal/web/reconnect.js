// Close code the service sends when the shell itself ended. Reconnecting then
// would start a new shell each time one exits, so the page waits to be asked.
export const SHELL_EXITED = 4000;

const FIRST_RETRY_MS = 1000;
const SLOWEST_RETRY_MS = 10000;

/** What to do once a connection has closed. */
export function afterClose({ closeCode, visible }) {
  if (closeCode === SHELL_EXITED) return "ended";
  // A closed pane does not hold a connection open; it reconnects when shown.
  return visible ? "retry" : "wait";
}

/** Delay before retry number `attempt`, counting from zero. */
export function retryDelayMs(attempt) {
  return Math.min(FIRST_RETRY_MS * 2 ** Math.max(0, attempt), SLOWEST_RETRY_MS);
}

/** The origin of the Remote shell embedding `<label>--<project>.<remote-host>`. */
export function embedderOrigin({ protocol, host }) {
  const separator = host.indexOf(".");
  if (separator <= 0 || !host.slice(0, separator).includes("--")) return null;
  return `${protocol}//${host.slice(separator + 1)}`;
}
