import { useEffect, useRef } from "preact/hooks";
import type { ChatStatus } from "../../../models/chat";
import { chatApi } from "../../../api/chatApi";
import { isPushPageFocused } from "../../stores/push/pushPageFocus";

export function useChatReadMarker({
  chatId,
  eventCount,
  status,
}: {
  chatId: string;
  eventCount: number;
  status: ChatStatus;
}) {
  const readMarkerRef = useRef("");

  useEffect(() => {
    if (status !== "ready") return;
    const markWhenVisible = () => {
      if (!isPushPageFocused()) return;
      const key = `${chatId}:${eventCount}`;
      if (readMarkerRef.current === key) return;
      readMarkerRef.current = key;
      void chatApi.markRead(chatId).catch(() => {
        // A failed request should be retried when the tab regains focus.
        if (readMarkerRef.current === key) readMarkerRef.current = "";
      });
    };
    markWhenVisible();
    window.addEventListener("focus", markWhenVisible);
    document.addEventListener("visibilitychange", markWhenVisible);
    return () => {
      window.removeEventListener("focus", markWhenVisible);
      document.removeEventListener("visibilitychange", markWhenVisible);
    };
  }, [chatId, eventCount, status]);
}
