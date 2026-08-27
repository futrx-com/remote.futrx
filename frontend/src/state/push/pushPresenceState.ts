// Tells the server which chat this user has on screen.
//
// The service worker already stays quiet about a chat it can see for itself,
// but that only covers the browser it runs in: the phone in your pocket has no
// idea you are reading the answer on a laptop. This is the half of the signal
// that travels, and it is what keeps every other device quiet while you are in
// the conversation.
//
// It reports regardless of whether *this* device is subscribed to push — the
// laptop you are typing on may have no subscription at all, and it is still
// the reason the phone should stay silent.

import { pushApi } from "../../api/pushApi";
import { PUSH_PRESENCE_HEARTBEAT_MS } from "../../config/push";
import { isPushPageFocused } from "./pushPageFocus";

/**
 * Keeps the server's idea of what this client is watching in step with what is
 * actually on screen. The claim and its heartbeat change together in one
 * place, so a repeat can never outlive the claim it was repeating.
 */
class PushPresenceState {
  /** Identifies this client for the life of the page. */
  readonly #clientId = this.#createClientId();
  /** The chat this client is showing, whether or not the user is looking. */
  #onScreen: string | null = null;
  /** The claim the server currently believes, so repeats stay cheap. */
  #claimed: string | null = null;
  /** Orders requests even when the network completes them out of order. */
  #revision = 0;
  #heartbeatTimer: number | undefined;
  #isListening = false;

  /**
   * Reports the chat on screen, or null when the app shows something else.
   * Safe to repeat: only a changed claim talks to the server.
   */
  setWatchedChat(chatId: string | null): void {
    this.#onScreen = chatId;
    this.#listen();
    this.#sync();
  }

  /** The chat the user counts as watching: in the app, and looking at it. */
  #chatInFocus(): string | null {
    if (!this.#onScreen || typeof document === "undefined") return null;
    // A visible but unfocused window is one the user left behind for another
    // app, which is exactly when they do want the notification.
    return isPushPageFocused() ? this.#onScreen : null;
  }

  #sync = (): void => {
    this.#claim(this.#chatInFocus());
  };

  /**
   * The only place the claim changes. Restarting the heartbeat here is what
   * keeps "a claim is being repeated" and "there is a claim" the same fact.
   */
  #claim(chatId: string | null): void {
    if (chatId === this.#claimed) return;
    this.#claimed = chatId;
    this.#restartHeartbeat();
    // Withdrawals ride keepalive: they often fire as the page is going away,
    // and a cancelled one would leave the user silenced until the claim
    // expires.
    void this.#send(chatId, chatId === null);
  }

  #restartHeartbeat(): void {
    if (this.#heartbeatTimer !== undefined) {
      clearInterval(this.#heartbeatTimer);
      this.#heartbeatTimer = undefined;
    }
    if (!this.#claimed) return;
    this.#heartbeatTimer = window.setInterval(() => {
      if (this.#claimed) void this.#send(this.#claimed, false);
    }, PUSH_PRESENCE_HEARTBEAT_MS);
  }

  async #send(chatId: string | null, keepalive: boolean): Promise<void> {
    const revision = ++this.#revision;
    try {
      await pushApi.presence(
        { chatId: chatId ?? "", clientId: this.#clientId, revision },
        keepalive
      );
    } catch {
      // A lost heartbeat costs one notification the user did not need, never
      // one they did, so there is nothing here worth surfacing or retrying.
    }
  }

  #listen(): void {
    if (this.#isListening || typeof window === "undefined") return;
    this.#isListening = true;

    document.addEventListener("visibilitychange", this.#sync);
    window.addEventListener("focus", this.#sync);
    window.addEventListener("blur", this.#sync);
    // The last beat that reliably fires on mobile, where a backgrounded tab
    // may simply never be resumed.
    window.addEventListener("pagehide", () => this.#claim(null));
  }

  #createClientId(): string {
    if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
      return crypto.randomUUID();
    }
    return Math.random().toString(36).slice(2) + Date.now().toString(36);
  }
}

export const pushPresenceState = new PushPresenceState();
