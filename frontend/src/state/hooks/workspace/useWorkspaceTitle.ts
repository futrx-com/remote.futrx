import { useEffect, useState } from "preact/hooks";
import type { ChatMeta } from "../../../models/chat";
import type { WorkspaceView } from "../../../models/workspace";
import { browserTitleForWorkspace, BASE_TITLE } from "../../../services/workspace/browserTitleService";
import { isPushPageFocused } from "../../stores/push/pushPageFocus";

export function useWorkspaceTitle({
  chats,
  activeChatId,
  view,
  enabled,
  loaded,
}: {
  chats: ChatMeta[];
  activeChatId: string | null;
  view: WorkspaceView;
  enabled: boolean;
  loaded: boolean;
}): void {
  const [focused, setFocused] = useState(isPushPageFocused);

  useEffect(() => {
    const refreshFocus = () => setFocused(isPushPageFocused());
    window.addEventListener("focus", refreshFocus);
    window.addEventListener("blur", refreshFocus);
    document.addEventListener("visibilitychange", refreshFocus);
    return () => {
      window.removeEventListener("focus", refreshFocus);
      window.removeEventListener("blur", refreshFocus);
      document.removeEventListener("visibilitychange", refreshFocus);
    };
  }, []);

  useEffect(() => {
    if (!enabled || !loaded) return;
    document.title = browserTitleForWorkspace(chats, activeChatId, view, focused);
  }, [chats, activeChatId, view, focused, enabled, loaded]);

  useEffect(() => () => { document.title = BASE_TITLE; }, []);
}
