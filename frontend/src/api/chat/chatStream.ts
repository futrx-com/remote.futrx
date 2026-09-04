import { ReconnectingJsonWebSocket } from "../../transport/reconnectingJsonSocket";
import { webSocketUrl } from "../../transport/webSocketUrl";
import type { ChatEvent } from "../../models/chat";
import type { ChatStream, ChatStreamCallbacks } from "../../types/chatApi";
import { WEB_SOCKET_ROUTES } from "../../config/routes";
import { CHAT_STREAM_MESSAGE_TYPES } from "../../config/api";
import type { ChatInteractionIntent } from "../../models/chatInteraction";
import { chatInteractionService } from "../../services/chat/chatInteractionService.ts";

export function openChatStream(
  chatId: string,
  latestSeq: () => number,
  callbacks: ChatStreamCallbacks
): ChatStream {
  const stream = new ReconnectingChatStream(chatId, latestSeq, callbacks);
  stream.open();
  return stream;
}

class ReconnectingChatStream implements ChatStream {
  readonly #connection: ReconnectingJsonWebSocket<ChatEvent>;

  constructor(
    chatId: string,
    latestSeq: () => number,
    callbacks: ChatStreamCallbacks
  ) {
    this.#connection = new ReconnectingJsonWebSocket({
      resolveUrl: () => webSocketUrl(WEB_SOCKET_ROUTES.chat(chatId, latestSeq())),
      onOpen: callbacks.onOpen,
      onMessage: callbacks.onEvent,
      onClose: callbacks.onClose,
    });
  }

  get isOpen(): boolean {
    return this.#connection.isOpen;
  }

  open(): void {
    this.#connection.start();
  }

  sendPrompt(text: string, clientId?: string): boolean {
    return this.#connection.send({ type: CHAT_STREAM_MESSAGE_TYPES.prompt, text, clientId });
  }

  cancel(): boolean {
    return this.#connection.send({ type: CHAT_STREAM_MESSAGE_TYPES.cancel });
  }

  respondInteraction(
    interactionId: string,
    method: string,
    intent: ChatInteractionIntent
  ): boolean {
    const response = chatInteractionService.encodeResponse(method, intent);
    return this.#connection.send({
      type: CHAT_STREAM_MESSAGE_TYPES.interactionResponse,
      interactionId,
      ...response,
    });
  }

  close(): void {
    this.#connection.stop();
  }
}
