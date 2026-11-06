import { afterFailure, retryDelayMs } from "./reconnect.js";
import { decodeOutput } from "./shellApi.js";

/**
 * Keeps one drawer attached to its chat's shell. Output is read by offset in
 * a loop of waiting requests, input is sent in order with keystrokes typed
 * meanwhile batched into the next request, and a lost connection reopens the
 * same shell.
 */
export function createShellConnection({
  api,
  session,
  cwd,
  size,
  onOutput,
  onReset,
  onEnded,
  onConnected,
  onStatus,
  timers = globalThis,
}) {
  let generation = null;
  let offset = -1;
  // Bumped on every (re)connection; a loop from an older one stops.
  let token = 0;
  let controller = null;
  let attempt = 0;
  let retryTimer = null;
  let ended = false;
  let connecting = false;
  let visible = false;
  let started = false;
  let disposed = false;
  let pending = "";
  let sending = false;

  function report(status) {
    if (!disposed) onStatus(status);
  }

  function schedule(delay) {
    timers.clearTimeout(retryTimer);
    retryTimer = timers.setTimeout(() => {
      retryTimer = null;
      connect();
    }, delay);
  }

  async function connect() {
    if (disposed) return;
    timers.clearTimeout(retryTimer);
    retryTimer = null;
    controller?.abort();
    controller = new AbortController();
    const current = ++token;
    ended = false;
    generation = null;
    pending = "";
    report(attempt === 0 ? "connecting" : "reconnecting");
    connecting = true;
    try {
      const { cols, rows } = size();
      const attached = await api.attach({ session, cwd, cols, rows });
      if (current !== token) return;
      connecting = false;
      generation = attached.generation;
      offset = -1;
      attempt = 0;
      report("connected");
      // An existing shell keeps its last size until this pane reports its own.
      onConnected();
      await readLoop(current, controller.signal);
    } catch (error) {
      if (current !== token) return;
      connecting = false;
      lost(error);
    }
  }

  async function readLoop(current, signal) {
    while (current === token) {
      const output = await api.read({ session, generation, offset }, signal);
      if (current !== token) return;
      if (output.reset) onReset();
      const data = decodeOutput(output.data);
      if (data.length > 0) onOutput(data);
      offset = output.offset;
      if (output.exited) {
        token++;
        generation = null;
        ended = true;
        report("ended");
        onEnded();
        return;
      }
    }
  }

  function lost(error) {
    if (disposed || error?.name === "AbortError") return;
    token++;
    generation = null;
    const step = afterFailure({ ended, visible });
    report("reconnecting");
    if (step !== "retry") return;
    // A backend that restarted no longer knows the session: open the chat's
    // shell again straight away rather than after a backoff.
    schedule(error?.status === 404 && attempt === 0 ? 0 : retryDelayMs(attempt));
    attempt += 1;
  }

  function flush() {
    if (sending || !pending || generation === null) return;
    const target = { session, generation };
    const data = pending;
    pending = "";
    sending = true;
    api.write(target, data)
      .catch((error) => {
        if (target.generation === generation) lost(error);
      })
      .finally(() => {
        sending = false;
        flush();
      });
  }

  return {
    /** Text typed into the terminal. */
    input(data) {
      if (ended) {
        if (data === "\r") {
          attempt = 0;
          connect();
        }
        return;
      }
      if (generation === null) return;
      pending += data;
      flush();
    },
    resize({ cols, rows }) {
      if (generation === null) return;
      api.resize({ session, generation, cols, rows }).catch(() => {});
    },
    /** The pane was shown or hidden. */
    setVisible(shown) {
      visible = shown;
      if (!shown) {
        // Stop retrying behind a closed pane; a working connection is kept.
        timers.clearTimeout(retryTimer);
        retryTimer = null;
        return;
      }
      this.resume();
    },
    /** Reconnect now if the pane is showing a connection that was lost. */
    resume() {
      if (!started || !visible || generation !== null || connecting || ended || disposed) return;
      attempt = 0;
      connect();
    },
    start() {
      started = true;
      connect();
    },
    dispose() {
      disposed = true;
      token++;
      controller?.abort();
      timers.clearTimeout(retryTimer);
    },
  };
}
