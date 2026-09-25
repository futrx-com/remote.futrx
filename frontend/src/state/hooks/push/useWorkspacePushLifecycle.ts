import { useEffect } from "preact/hooks";

import type { WorkspaceView } from "../../../models/workspace";
import { pushNotificationStore } from "../../stores/push/pushNotificationStore";
import { pushPresenceStore } from "../../stores/push/pushPresenceStore";
import { closeNotificationsOfWatchedChat } from "../../stores/push/pushTrayCleanup";
import { usePushDeviceRestore } from "./usePushDeviceRestore";

interface WorkspacePushLifecycleOptions {
  /** The signed-in account, or "" while the session is not established. */
  account: string;
  activeChatId: string | null;
  view: WorkspaceView;
  openChat: (chatId: string) => void;
}

/** Keeps this device's registration, worker routing, and presence in sync. */
export function useWorkspacePushLifecycle({
  account,
  activeChatId,
  view,
  openChat,
}: WorkspacePushLifecycleOptions): void {
  // Register the worker on every boot so a deployed sw.js replaces the
  // installed one, and route notification taps into chat selection.
  useEffect(() => {
    pushNotificationStore.getState().connect((chatId) => {
      if (chatId) openChat(chatId);
    });
  }, [openChat]);

  usePushDeviceRestore(account);

  // Say which chat is on screen, so nothing interrupts the user about the one
  // they are already watching. The worker covers this browser; the server
  // covers the user's other devices, which the worker cannot see.
  useEffect(() => {
    const onScreen = view === "chat" ? activeChatId : null;
    pushNotificationStore.getState().setVisibleChat(onScreen);
    pushPresenceStore.getState().setWatchedChat(onScreen);
  }, [activeChatId, view]);

  // Clear a chat's notifications once the user is actually looking at it.
  useEffect(() => closeNotificationsOfWatchedChat(), []);
}
