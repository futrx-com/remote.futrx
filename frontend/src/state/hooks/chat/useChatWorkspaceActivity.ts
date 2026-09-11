import { useEffect } from "preact/hooks";
import { chatActivityApi } from "../../../api/chat/chatActivityApi";

const WORKSPACE_ACTIVITY_INTERVAL_MS = 30_000;

/**
 * Keeps a visible project chat's workspace lease fresh. Failures stay silent:
 * ordinary workspace operations remain the user-facing error boundary.
 */
export function useChatWorkspaceActivity(chatId: string, enabled: boolean): void {
  useEffect(() => {
    if (!enabled) return;

    let pending = false;
    const touch = async () => {
      if (document.visibilityState !== "visible" || pending) return;
      pending = true;
      try {
        await chatActivityApi.touch(chatId);
      } catch {
        // Normal workspace requests surface connectivity and restore errors.
      } finally {
        pending = false;
      }
    };

    void touch();
    const timer = window.setInterval(() => void touch(), WORKSPACE_ACTIVITY_INTERVAL_MS);
    document.addEventListener("visibilitychange", touch);
    return () => {
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", touch);
    };
  }, [chatId, enabled]);
}
