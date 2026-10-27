import type { ChatEvent } from "../models/chat";
import type { ChatInteractionIntent } from "../models/chatInteraction";

export type ChatInteractionResponder = (
  interactionId: string,
  method: string,
  intent: ChatInteractionIntent
) => boolean;

export interface ChatStream {
  readonly isOpen: boolean;
  sendPrompt(text: string, clientId?: string): boolean;
  cancel(): boolean;
  respondInteraction(
    interactionId: string,
    method: string,
    intent: ChatInteractionIntent
  ): boolean;
  close(): void;
}

export interface ChatStreamCallbacks {
  onOpen: () => void;
  onEvent: (event: ChatEvent) => void;
  onClose: () => void;
}
