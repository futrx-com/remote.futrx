import { pushServiceWorkerApi } from "../../../api/pushServiceWorkerApi.ts";
import { pushPresenceStore } from "./pushPresenceStore.ts";

/**
 * Closes a chat's notifications once the user is watching it. iOS never
 * clears them on its own, so every turn would otherwise leave one in the
 * tray. Presence already decides what "watching" means — on screen, visible,
 * and focused — so this follows its claim rather than tracking focus again.
 * Returns the unsubscribe function.
 */
export function closeNotificationsOfWatchedChat(): () => void {
  // A chat claimed before this subscribes — the one a tapped notification
  // cold-started the app on — is being watched too.
  closeChatNotifications(pushPresenceStore.getState().claimedChatId);
  return pushPresenceStore.subscribe((state, previous) => {
    if (state.claimedChatId !== previous.claimedChatId) {
      closeChatNotifications(state.claimedChatId);
    }
  });
}

function closeChatNotifications(chatId: string | null): void {
  if (chatId) void pushServiceWorkerApi.closeChatNotifications(chatId);
}
