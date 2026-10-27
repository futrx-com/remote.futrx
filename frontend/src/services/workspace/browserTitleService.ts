import type { ChatMeta } from "../../models/chat";
import type { WorkspaceView } from "../../models/workspace";

const BASE_TITLE = "remote.futrx";

export function browserTitleForWorkspace(
  chats: ChatMeta[],
  activeChatId: string | null,
  view: WorkspaceView,
  focused: boolean,
): string {
  const unread = chats.filter((chat) =>
    !chat.running &&
    (chat.lastMessageAt || 0) > (chat.lastReadAt || 0) &&
    !(focused && view === "chat" && chat.id === activeChatId)
  ).length;
  return unread ? `(${unread}) ${BASE_TITLE}` : BASE_TITLE;
}

export { BASE_TITLE };
