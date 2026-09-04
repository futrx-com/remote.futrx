import type { ComponentChildren } from "preact";
import { createContext } from "preact";
import { useCallback, useContext, useEffect, useLayoutEffect, useMemo, useReducer } from "preact/hooks";
import type { ChatMeta } from "../../models/chat";
import type { ProjectMeta } from "../../models/project";
import { chatApi } from "../../api/chatApi";
import { createChatInput } from "./createChatInput";
import { projectApi } from "../../api/projectApi";
import { useWorkspaceData } from "../hooks/workspace/useWorkspaceData";
import { useWorkspacePushLifecycle } from "../hooks/push/useWorkspacePushLifecycle";
import { useUserSettingsContext } from "./UserSettingsContext";
import type { WorkspaceUiState } from "../../models/workspace";
import { workspaceUiState } from "./workspaceUiState";
import { workspaceSidebarService } from "../../services/workspace/workspaceSidebarService.ts";
import { agentCapabilityCatalogStore } from "../stores/agents/agentCapabilityCatalogStore";
import { takePushNotificationChatId } from "./pushNotificationNavigation";
import { useAuthContext } from "./AuthContext";

interface WorkspaceContextValue {
  chats: ChatMeta[];
  projects: ProjectMeta[];
  activeChat: ChatMeta | null;
  /** False until the first workspace snapshot lands. An empty list before that
   *  means "not known yet" — surfaces must show placeholders, not empty states. */
  loaded: boolean;
  ui: WorkspaceUiState;
  selectChat: (chatId: string | null) => void;
  openSidebar: () => void;
  closeSidebar: () => void;
  showChat: () => void;
  showSettings: () => void;
  showProjectContainers: (projectId: string | null) => void;
  openCreateProject: () => void;
  closeCreateProject: () => void;
  createProject: (name: string) => Promise<ProjectMeta>;
  createChat: (projectId?: string) => Promise<ChatMeta>;
  deleteChat: (chatId: string) => Promise<void>;
  forkChat: (chatId: string) => Promise<ChatMeta>;
  deleteProject: (projectId: string) => Promise<void>;
  reorderProjects: (projectIds: string[]) => Promise<void>;
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);

export function WorkspaceProvider({
  enabled,
  children,
}: {
  enabled: boolean;
  children: ComponentChildren;
}) {
  ////////////////
  // Local State
  ////////////////
  const data = useWorkspaceData(enabled);
  const { auth } = useAuthContext();
  const { settings } = useUserSettingsContext();
  const [ui, dispatch] = useReducer(
    workspaceUiState.reduce,
    null,
    () => workspaceUiState.createInitial(takePushNotificationChatId())
  );
  const activeChat = workspaceSidebarService.activeChat(data.chats, ui.activeChatId);
  const capabilityUserId = auth.email || auth.adminEmail || "anonymous";
  const activeCapabilityProjectId = activeChat?.projectId;

  ////////////////
  // Handlers
  ////////////////
  const openPushChat = useCallback((chatId: string) => {
    dispatch({ type: "select-chat", chatId });
  }, []);

  const activateNewChat = useCallback((chat: ChatMeta): ChatMeta => {
    data.seedChat(chat);
    dispatch({ type: "select-chat", chatId: chat.id });
    return chat;
  }, [data.seedChat]);

  const createProject = useCallback(async (name: string): Promise<ProjectMeta> => {
    const project = await projectApi.create(name);
    return project;
  }, []);

  const createChat = useCallback(async (projectId?: string): Promise<ChatMeta> => {
    const chat = await chatApi.create(createChatInput(settings, projectId));
    return activateNewChat(chat);
  }, [settings, activateNewChat]);

  const deleteChat = useCallback(async (chatId: string) => {
    await chatApi.delete(chatId);
  }, []);

  const forkChat = useCallback(async (chatId: string): Promise<ChatMeta> => {
    const chat = await chatApi.fork(chatId);
    return activateNewChat(chat);
  }, [activateNewChat]);

  const deleteProject = useCallback(async (projectId: string) => {
    await projectApi.delete(projectId);
    agentCapabilityCatalogStore.getState().removeProject(capabilityUserId, projectId);
  }, [capabilityUserId]);

  const reorderProjects = useCallback(async (projectIds: string[]) => {
    await projectApi.reorder(projectIds);
  }, []);

  // Dispatch-only commands. preact creates `dispatch` once, so these close over
  // nothing that can go stale and need no dependencies.
  const selectChat = useCallback((chatId: string | null) => {
    dispatch({ type: "select-chat", chatId });
  }, []);
  const openSidebar = useCallback(() => dispatch({ type: "open-sidebar" }), []);
  const closeSidebar = useCallback(() => dispatch({ type: "close-sidebar" }), []);
  const showChat = useCallback(() => dispatch({ type: "show-chat" }), []);
  const showSettings = useCallback(() => dispatch({ type: "show-settings" }), []);
  const showProjectContainers = useCallback((projectId: string | null) => {
    dispatch({ type: "show-project-containers", projectId });
  }, []);
  const openCreateProject = useCallback(() => dispatch({ type: "open-create-project" }), []);
  const closeCreateProject = useCallback(() => dispatch({ type: "close-create-project" }), []);

  ////////////////
  // Effects
  ////////////////
  useEffect(() => {
    if (!enabled || !activeChat) return;
    void agentCapabilityCatalogStore.getState()
      .load(capabilityUserId, activeCapabilityProjectId)
      .catch(() => undefined);
  }, [enabled, capabilityUserId, activeCapabilityProjectId, activeChat?.id]);

  useWorkspacePushLifecycle({
    activeChatId: ui.activeChatId,
    view: ui.view,
    openChat: openPushChat,
  });

  useEffect(() => {
    const chatId = workspaceSidebarService.initialChatId(enabled, ui.activeChatId, data.chats);
    if (chatId) dispatch({ type: "select-chat", chatId });
  }, [data.chats, enabled, ui.activeChatId]);

  // Layout effect, not a passive one: the render that drops the chat from the
  // list already resolves activeChat to null, so a passive effect would let the
  // browser paint the "no chat selected" screen before the handover lands.
  useLayoutEffect(() => {
    // Wait for the first snapshot: a chat id handed over by a notification tap
    // would otherwise be discarded against a not-yet-populated list.
    if (!data.loaded) return;
    if (workspaceSidebarService.isActiveChatMissing(data.chats, ui.activeChatId)) {
      // Hand straight over to the next chat instead of clearing the selection:
      // clearing renders the "no chat selected" empty state for the one frame
      // before the initial-chat effect picks a replacement, which reads as a
      // flash of the New project screen after deleting a chat.
      dispatch({
        type: "select-chat",
        chatId: workspaceSidebarService.replacementChatId(data.chats),
      });
    }
  }, [data.chats, data.loaded, ui.activeChatId]);

  ////////////////
  // Context Value
  ////////////////
  // preact force-renders every subscriber whenever the provider's value fails a
  // `!=` check, so a fresh literal here repainted the whole workspace subtree on
  // any render of this provider — including ones driven by upstream auth or
  // settings ticks this tree does not read.
  const value = useMemo<WorkspaceContextValue>(() => ({
    chats: data.chats,
    projects: data.projects,
    activeChat,
    loaded: data.loaded,
    ui,
    selectChat,
    openSidebar,
    closeSidebar,
    showChat,
    showSettings,
    showProjectContainers,
    openCreateProject,
    closeCreateProject,
    createProject,
    createChat,
    deleteChat,
    forkChat,
    deleteProject,
    reorderProjects,
  }), [
    data.chats,
    data.projects,
    data.loaded,
    activeChat,
    ui,
    selectChat,
    openSidebar,
    closeSidebar,
    showChat,
    showSettings,
    showProjectContainers,
    openCreateProject,
    closeCreateProject,
    createProject,
    createChat,
    deleteChat,
    forkChat,
    deleteProject,
    reorderProjects,
  ]);

  return (
    <WorkspaceContext.Provider value={value}>
      {children}
    </WorkspaceContext.Provider>
  );
}

export function useWorkspaceContext(): WorkspaceContextValue {
  const value = useContext(WorkspaceContext);
  if (!value) throw new Error("useWorkspaceContext must be used inside WorkspaceProvider");
  return value;
}
