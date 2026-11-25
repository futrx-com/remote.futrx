// A request, made outside a chat, to open it at a message: the sidebar's
// message search asks, and the chat consumes it once its history is ready.

import type { ConversationSearchHit } from "../../api/chat/chatTranscriptApi";

export interface ChatJumpRequest {
  chatId: string;
  /** The search to show in the chat's find bar. */
  query: string;
  hit: ConversationSearchHit;
}

type Listener = () => void;

class ChatJumpRequestService {
  #pending: ChatJumpRequest | null = null;
  readonly #listeners = new Set<Listener>();

  request(jump: ChatJumpRequest): void {
    this.#pending = jump;
    this.#notify();
  }

  /** The pending request for `chatId`, without consuming it. */
  peek(chatId: string): ChatJumpRequest | null {
    return this.#pending?.chatId === chatId ? this.#pending : null;
  }

  /** Consume `jump` if it is still the pending request. */
  take(jump: ChatJumpRequest): boolean {
    if (this.#pending !== jump) return false;
    this.#pending = null;
    this.#notify();
    return true;
  }

  subscribe(listener: Listener): () => void {
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  }

  #notify(): void {
    for (const listener of this.#listeners) listener();
  }
}

export const chatJumpRequestService = new ChatJumpRequestService();
