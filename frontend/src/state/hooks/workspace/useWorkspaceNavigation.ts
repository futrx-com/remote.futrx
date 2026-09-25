import { useCallback, useEffect, useLayoutEffect, useReducer } from "preact/hooks";
import type { ChatMeta } from "../../../models/chat";
import type { SettingsTab } from "../../../models/workspace";
import { workspaceSidebarService } from "../../../services/workspace/workspaceSidebarService.ts";
import { currentWorkspaceRoute, navigateWorkspace } from "../../context/workspaceNavigation";
import { workspaceUiState } from "../../context/workspaceUiState";

/** Keeps workspace UI state and the browser address in sync. */
export function useWorkspaceNavigation(chats: ChatMeta[], loaded: boolean, enabled: boolean) {
  const [ui, dispatch] = useReducer(
    workspaceUiState.reduce,
    null,
    () => {
      const route = currentWorkspaceRoute();
      return workspaceUiState.createInitial(route.chatId, route.tab, route.view);
    }
  );

  const selectChat = useCallback((chatId: string | null) => {
    navigateWorkspace({ view: "chat", chatId, tab: "appearance" });
    dispatch({ type: "select-chat", chatId });
  }, []);
  const openSidebar = useCallback(() => dispatch({ type: "open-sidebar" }), []);
  const closeSidebar = useCallback(() => dispatch({ type: "close-sidebar" }), []);
  const showChat = useCallback(() => {
    navigateWorkspace({ view: "chat", chatId: ui.activeChatId, tab: ui.settingsTab });
    dispatch({ type: "show-chat" });
  }, [ui.activeChatId, ui.settingsTab]);
  const showSettings = useCallback(() => {
    navigateWorkspace({ view: "settings", chatId: null, tab: ui.settingsTab });
    dispatch({ type: "show-settings" });
  }, [ui.settingsTab]);
  const selectSettingsTab = useCallback((tab: SettingsTab) => {
    navigateWorkspace({ view: "settings", chatId: null, tab });
    dispatch({ type: "select-settings-tab", tab });
  }, []);
  const showProjectContainers = useCallback((projectId: string | null) => {
    navigateWorkspace({ view: "chat", chatId: null, tab: "appearance" });
    dispatch({ type: "show-project-containers", projectId });
  }, []);
  const openCreateProject = useCallback(() => dispatch({ type: "open-create-project" }), []);
  const closeCreateProject = useCallback(() => dispatch({ type: "close-create-project" }), []);

  useEffect(() => {
    const restore = () => {
      const route = currentWorkspaceRoute();
      dispatch({ type: "restore-route", chatId: route.chatId, view: route.view, tab: route.tab });
    };
    window.addEventListener("popstate", restore);
    return () => window.removeEventListener("popstate", restore);
  }, []);

  useEffect(() => {
    if (ui.view !== "chat" || !loaded) return;
    const chatId = workspaceSidebarService.initialChatId(enabled, ui.activeChatId, chats);
    if (chatId) {
      navigateWorkspace({ view: "chat", chatId, tab: ui.settingsTab }, true);
      dispatch({ type: "select-chat", chatId });
    }
  }, [chats, loaded, enabled, ui.activeChatId, ui.settingsTab, ui.view]);

  // Avoid painting an empty chat view between a deletion and its replacement.
  useLayoutEffect(() => {
    // Wait for the first snapshot before resolving a chat opened from a notification.
    if (!loaded || ui.view !== "chat") return;
    if (workspaceSidebarService.isActiveChatMissing(chats, ui.activeChatId)) {
      const replacement = workspaceSidebarService.replacementChatId(chats);
      navigateWorkspace({ view: "chat", chatId: replacement, tab: ui.settingsTab }, true);
      dispatch({ type: "select-chat", chatId: replacement });
    }
  }, [chats, loaded, ui.activeChatId, ui.settingsTab, ui.view]);

  return {
    ui,
    selectChat,
    openSidebar,
    closeSidebar,
    showChat,
    showSettings,
    selectSettingsTab,
    showProjectContainers,
    openCreateProject,
    closeCreateProject,
  };
}
