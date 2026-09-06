import { API_ROUTES } from "../../config/routes";
import { requestJson } from "../apiRequest";

export const chatActivityApi = {
  touch: (chatId: string) =>
    requestJson<void>("POST", API_ROUTES.chats.activity(chatId)),
};
