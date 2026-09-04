import { requestJson } from "../apiRequest";
import { openChatStream } from "./chatStream";
import type { ChatEventPage } from "../../models/chat";
import type { ChatStream, ChatStreamCallbacks } from "../../types/chatApi";
import { API_ROUTES } from "../../config/routes";

export const chatEventsApi = {
  rewind: (id: string, beforeT: number) =>
    requestJson<ChatEventPage>("POST", API_ROUTES.chats.rewind(id), { beforeT }),

  openStream: (
    id: string,
    latestSeq: () => number,
    callbacks: ChatStreamCallbacks
  ): ChatStream => openChatStream(id, latestSeq, callbacks),
};
