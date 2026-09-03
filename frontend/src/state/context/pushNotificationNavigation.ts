/**
 * Reads a chat id handed over by a cold-start notification tap and clears it
 * from the address bar, so reloading later does not jump back to it.
 */
export function takePushNotificationChatId(): string | null {
  if (typeof window === "undefined") return null;
  const params = new URLSearchParams(window.location.search);
  const chatId = params.get("chat");
  if (!chatId) return null;

  params.delete("chat");
  const query = params.toString();
  window.history.replaceState(
    null,
    "",
    window.location.pathname + (query ? `?${query}` : "") + window.location.hash
  );
  return chatId;
}
