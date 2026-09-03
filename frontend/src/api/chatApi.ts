import { requestJson } from "./apiRequest";
import { chatEventsApi } from "./chat/chatEventsApi";
import { chatFilesApi } from "./chat/chatFilesApi";
import { chatHistoryApi } from "./chat/chatHistoryApi";
import { chatScheduleApi } from "./chat/chatScheduleApi";
import { fetchTranscript } from "./chat/chatTranscriptApi";
import type { ChatMeta, CreateChatInput, UpdateChatInput } from "../models/chat";
import { API_ROUTES } from "../config/routes";

export const chatApi = {
  list: () => requestJson<ChatMeta[]>("GET", API_ROUTES.chats.collection),
  create: (body: CreateChatInput = {}) =>
    requestJson<ChatMeta>("POST", API_ROUTES.chats.collection, body),
  fetch: (id: string) =>
    requestJson<ChatMeta>("GET", API_ROUTES.chats.item(id)),
  update: (id: string, body: UpdateChatInput) =>
    requestJson<ChatMeta>("PATCH", API_ROUTES.chats.item(id), body),
  markRead: (id: string) =>
    requestJson<ChatMeta>("POST", API_ROUTES.chats.read(id), {}),
  markUnread: (id: string) =>
    requestJson<ChatMeta>("POST", API_ROUTES.chats.unread(id), {}),
  delete: (id: string) =>
    requestJson<{ ok: boolean }>("DELETE", API_ROUTES.chats.item(id)),
  fork: (id: string) =>
    requestJson<ChatMeta>("POST", API_ROUTES.chats.fork(id), {}),
  ...chatFilesApi,
  fetchTranscript,
  rewind: chatEventsApi.rewind,
  ...chatHistoryApi,
  ...chatScheduleApi,
  openStream: chatEventsApi.openStream,
};
