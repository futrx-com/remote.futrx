import type {
  WorkspaceUiAction,
  WorkspaceUiState,
} from "../../models/workspace";

class WorkspaceUiStateTransitions {
  // A notification tap on a cold start arrives as ?chat=<id>, so the first
  // render can open straight into that chat instead of the newest one.
  createInitial(requestedChatId: string | null = null, settingsTab: WorkspaceUiState["settingsTab"] = "appearance", view: WorkspaceUiState["view"] = "chat"): WorkspaceUiState {
    return {
      activeChatId: requestedChatId,
      containerProjectId: null,
      sidebarOpen: false,
      createProjectOpen: false,
      view,
      settingsTab,
    };
  }

  readonly reduce = (
    state: WorkspaceUiState,
    action: WorkspaceUiAction
  ): WorkspaceUiState => {
    switch (action.type) {
      case "select-chat":
        return {
          ...state,
          activeChatId: action.chatId,
          sidebarOpen: false,
          view: "chat",
        };
      case "open-sidebar":
        return { ...state, sidebarOpen: true };
      case "close-sidebar":
        return { ...state, sidebarOpen: false };
      case "open-create-project":
        return { ...state, createProjectOpen: true };
      case "close-create-project":
        return { ...state, createProjectOpen: false };
      case "show-chat":
        return { ...state, view: "chat" };
      case "show-settings":
        return { ...state, view: "settings", sidebarOpen: false };
      case "select-settings-tab":
        return { ...state, settingsTab: action.tab };
      case "restore-route":
        return {
          ...state,
          view: action.view,
          activeChatId: action.view === "chat" ? action.chatId : state.activeChatId,
          settingsTab: action.tab,
          sidebarOpen: false,
        };
      case "show-project-containers":
        return {
          ...state,
          containerProjectId: action.projectId,
          view: "project-containers",
          sidebarOpen: false,
        };
    }
  };
}

export const workspaceUiState = new WorkspaceUiStateTransitions();
