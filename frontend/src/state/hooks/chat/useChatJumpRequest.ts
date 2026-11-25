import { useEffect, useState } from "preact/hooks";
import {
  chatJumpRequestService,
  type ChatJumpRequest,
} from "../../../services/chat/chatJumpRequestService.ts";

/** The pending request to open `chatId` at a message, kept current. */
export function useChatJumpRequest(chatId: string): ChatJumpRequest | null {
  const [request, setRequest] = useState(() => chatJumpRequestService.peek(chatId));
  useEffect(() => {
    setRequest(chatJumpRequestService.peek(chatId));
    return chatJumpRequestService.subscribe(() => setRequest(chatJumpRequestService.peek(chatId)));
  }, [chatId]);
  return request;
}
