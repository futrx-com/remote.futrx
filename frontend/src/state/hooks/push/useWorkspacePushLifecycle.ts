import { useEffect } from "preact/hooks";

import type { WorkspaceView } from "../../workspace/workspaceUiState";
import { pushNotificationState } from "../../push/pushNotificationState";
import { pushPresenceState } from "../../push/pushPresenceState";
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
    pushNotificationState.connect((chatId) => {
      if (chatId) openChat(chatId);
    });
  }, [openChat]);

  usePushDeviceRestore(account);

  // Say which chat is on screen, so nothing interrupts the user about the one
  // they are already watching. The worker covers this browser; the server
  // covers the user's other devices, which the worker cannot see.
  useEffect(() => {
    const onScreen = view === "chat" ? activeChatId : null;
    pushNotificationState.setVisibleChat(onScreen);
    pushPresenceState.setWatchedChat(onScreen);
  }, [activeChatId, view]);
}
