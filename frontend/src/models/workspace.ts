import type { ChatMeta } from "./chat";
import type { ProjectMeta } from "./project";
import type { WorkspaceMessage } from "../types/workspaceApi";

/** What the workspace feed has delivered so far. */
export interface WorkspaceSnapshot {
  chats: ChatMeta[];
  projects: ProjectMeta[];
  /** False until the first snapshot lands. An empty list before that means
   *  "not known yet", not "none" — callers must not act on the difference. */
  loaded: boolean;
}

/** Opens the workspace feed and reports messages until the returned call. */
export type SubscribeToWorkspace = (
  onMessage: (message: WorkspaceMessage) => void,
) => () => void;

export interface WorkspaceStoreState {
  snapshot: WorkspaceSnapshot;
}

export interface WorkspaceStoreActions {
  setConnected: (connected: boolean) => void;
  /** Applies a chat this client just created, ahead of the server's own
   *  `chat.upsert` for it. */
  seedChat: (chat: ChatMeta) => void;
}

export type WorkspaceView = "chat" | "settings" | "project-containers";

export interface WorkspaceUiState {
  activeChatId: string | null;
  containerProjectId: string | null;
  sidebarOpen: boolean;
  createProjectOpen: boolean;
  view: WorkspaceView;
}

export type WorkspaceUiAction =
  | { type: "select-chat"; chatId: string | null }
  | { type: "open-sidebar" }
  | { type: "close-sidebar" }
  | { type: "open-create-project" }
  | { type: "close-create-project" }
  | { type: "show-chat" }
  | { type: "show-settings" }
  | { type: "show-project-containers"; projectId: string | null };

export type DropPosition = "before" | "after";

export interface ProjectSidebarNode {
  project: ProjectMeta;
  chats: ChatMeta[];
}

/** The project tree. Ranked search results are owned by the search state. */
export interface WorkspaceSidebarModel {
  visibleProjects: ProjectSidebarNode[];
  visibleLooseChats: ChatMeta[];
  totalChats: number;
  totalProjects: number;
}
