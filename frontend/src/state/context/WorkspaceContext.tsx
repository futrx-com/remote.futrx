import type { ComponentChildren } from "preact";
import { createContext } from "preact";
import { useCallback, useContext, useEffect, useMemo } from "preact/hooks";
import type { ChatMeta } from "../../models/chat";
import type { ProjectMeta } from "../../models/project";
import { chatApi } from "../../api/chatApi";
import { createChatInput } from "./createChatInput";
import { projectApi } from "../../api/projectApi";
import { useWorkspaceData } from "../hooks/workspace/useWorkspaceData";
import { useWorkspacePushLifecycle } from "../hooks/push/useWorkspacePushLifecycle";
import { useWorkspaceTitle } from "../hooks/workspace/useWorkspaceTitle";
import { useWorkspaceNavigation } from "../hooks/workspace/useWorkspaceNavigation";
import { useUserSettingsContext } from "./UserSettingsContext";
import type { SettingsTab, WorkspaceUiState } from "../../models/workspace";
import { workspaceSidebarService } from "../../services/workspace/workspaceSidebarService.ts";
import { agentCapabilityCatalogStore } from "../stores/agents/agentCapabilityCatalogStore";
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
  selectSettingsTab: (tab: SettingsTab) => void;
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
  const { auth, agentAuth } = useAuthContext();
  const { settings } = useUserSettingsContext();
  const {
    ui, selectChat, openSidebar, closeSidebar, showChat, showSettings,
    selectSettingsTab, showProjectContainers, openCreateProject, closeCreateProject,
  } = useWorkspaceNavigation(data.chats, data.loaded, enabled);
  const activeChat = workspaceSidebarService.activeChat(data.chats, ui.activeChatId);
  const account = auth.email || auth.adminEmail;
  const capabilityUserId = account || "anonymous";
  const activeCapabilityProjectId = activeChat?.projectId;

  ////////////////
  // Handlers
  ////////////////
  const activateNewChat = useCallback((chat: ChatMeta): ChatMeta => {
    data.seedChat(chat);
    selectChat(chat.id);
    return chat;
  }, [data.seedChat, selectChat]);

  const createProject = useCallback(async (name: string): Promise<ProjectMeta> => {
    const project = await projectApi.create(name);
    return project;
  }, []);

  const createChat = useCallback(async (projectId?: string): Promise<ChatMeta> => {
    const chat = await chatApi.create(createChatInput(settings, projectId, agentAuth.providers));
    return activateNewChat(chat);
  }, [settings, agentAuth.providers, activateNewChat]);

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
    account: enabled ? account : "",
    activeChatId: ui.activeChatId,
    view: ui.view,
    openChat: selectChat,
  });

  useWorkspaceTitle({
    chats: data.chats,
    activeChatId: ui.activeChatId,
    view: ui.view,
    enabled,
    loaded: data.loaded,
  });

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
    selectSettingsTab,
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
    selectSettingsTab,
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
