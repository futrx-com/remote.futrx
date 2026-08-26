import type { ComponentChildren } from "preact";
import { createContext } from "preact";
import { useCallback, useContext, useEffect, useReducer } from "preact/hooks";
import type { ChatMeta, CreateChatInput } from "../../models/chat";
import type { ProjectMeta } from "../../models/project";
import { chatApi } from "../../api/chatApi";
import { projectApi } from "../../api/projectApi";
import { useWorkspaceData } from "../hooks/workspace/useWorkspaceData";
import { useWorkspacePushLifecycle } from "../hooks/push/useWorkspacePushLifecycle";
import { useUserSettingsContext } from "./UserSettingsContext";
import {
  workspaceUiState,
  type WorkspaceUiState,
} from "../workspace/workspaceUiState";
import { workspaceSidebarState } from "../workspace/workspaceSidebarState";
import { agentCapabilityCatalogStore } from "../agents/agentCapabilityCatalog";
import { takePushNotificationChatId } from "../push/pushNotificationNavigation";
import { useAuthContext } from "./AuthContext";

interface WorkspaceContextValue {
  chats: ChatMeta[];
  projects: ProjectMeta[];
  activeChat: ChatMeta | null;
  ui: WorkspaceUiState;
  selectChat: (chatId: string | null) => void;
  openSidebar: () => void;
  closeSidebar: () => void;
  showChat: () => void;
  showSettings: () => void;
  showProjectContainers: (projectId: string | null) => void;
  openCreateProject: () => void;
  closeCreateProject: () => void;
  createProject: (name: string, gitUrl?: string) => Promise<ProjectMeta>;
  createChat: (projectId?: string) => Promise<ChatMeta>;
  deleteChat: (chatId: string) => Promise<void>;
  forkChat: (chatId: string) => Promise<ChatMeta>;
  deleteProject: (projectId: string) => Promise<void>;
  reorderProjects: (projectIds: string[]) => Promise<void>;
  startProject: (projectId: string) => Promise<void>;
  stopProject: (projectId: string) => Promise<void>;
}

const WorkspaceContext = createContext<WorkspaceContextValue | null>(null);

export function WorkspaceProvider({
  enabled,
  children,
}: {
  enabled: boolean;
  children: ComponentChildren;
}) {
  const data = useWorkspaceData(enabled);
  const { auth } = useAuthContext();
  const { settings } = useUserSettingsContext();
  const [ui, dispatch] = useReducer(
    workspaceUiState.reduce,
    null,
    () => workspaceUiState.createInitial(takePushNotificationChatId())
  );
  const activeChat = workspaceSidebarState.activeChat(data.chats, ui.activeChatId);
  const capabilityUserId = auth.email || auth.adminEmail || "anonymous";
  const activeCapabilityProjectId = activeChat?.projectId;

  useEffect(() => {
    if (!enabled || !activeChat) return;
    void agentCapabilityCatalogStore
      .load(capabilityUserId, activeCapabilityProjectId)
      .catch(() => undefined);
  }, [enabled, capabilityUserId, activeCapabilityProjectId, activeChat?.id]);

  const openPushChat = useCallback((chatId: string) => {
    dispatch({ type: "select-chat", chatId });
  }, []);
  useWorkspacePushLifecycle({
    activeChatId: ui.activeChatId,
    view: ui.view,
    openChat: openPushChat,
  });

  useEffect(() => {
    const chatId = workspaceSidebarState.initialChatId(enabled, ui.activeChatId, data.chats);
    if (chatId) dispatch({ type: "select-chat", chatId });
  }, [data.chats, enabled, ui.activeChatId]);

  useEffect(() => {
    // Wait for the first snapshot: a chat id handed over by a notification tap
    // would otherwise be discarded against a not-yet-populated list.
    if (!data.loaded) return;
    if (workspaceSidebarState.isActiveChatMissing(data.chats, ui.activeChatId)) {
      dispatch({ type: "select-chat", chatId: null });
    }
  }, [data.chats, data.loaded, ui.activeChatId]);

  async function createProject(name: string, gitUrl?: string): Promise<ProjectMeta> {
    const project = await projectApi.create(name, gitUrl);
    return project;
  }

  async function createChat(projectId?: string): Promise<ChatMeta> {
    const input: CreateChatInput = {
      provider: settings.chat.provider,
      model: settings.chat.model,
      mode: settings.chat.mode,
      reasoningEffort: settings.chat.reasoningEffort,
      serviceTier: settings.chat.serviceTier,
      ...(projectId ? { projectId } : {}),
    };
    const chat = await chatApi.create(input);
    dispatch({ type: "select-chat", chatId: chat.id });
    return chat;
  }

  async function deleteChat(chatId: string) {
    await chatApi.delete(chatId);
  }

  async function forkChat(chatId: string): Promise<ChatMeta> {
    const chat = await chatApi.fork(chatId);
    dispatch({ type: "select-chat", chatId: chat.id });
    return chat;
  }

  async function deleteProject(projectId: string) {
    await projectApi.delete(projectId);
    agentCapabilityCatalogStore.removeProject(capabilityUserId, projectId);
  }

  async function reorderProjects(projectIds: string[]) {
    await projectApi.reorder(projectIds);
  }

  async function startProject(projectId: string) {
    await projectApi.start(projectId);
    // The sidebar Start command requests a probe inside the running container
    // instead of intentionally reusing a pre-start catalog. Other lifecycle
    // surfaces must invalidate separately or leave refresh to the user.
    agentCapabilityCatalogStore.invalidateProject(capabilityUserId, projectId);
  }

  async function stopProject(projectId: string) {
    await projectApi.stop(projectId);
  }

  return (
    <WorkspaceContext.Provider
      value={{
        chats: data.chats,
        projects: data.projects,
        activeChat,
        ui,
        selectChat: (chatId) => dispatch({ type: "select-chat", chatId }),
        openSidebar: () => dispatch({ type: "open-sidebar" }),
        closeSidebar: () => dispatch({ type: "close-sidebar" }),
        showChat: () => dispatch({ type: "show-chat" }),
        showSettings: () => dispatch({ type: "show-settings" }),
        showProjectContainers: (projectId) =>
          dispatch({ type: "show-project-containers", projectId }),
        openCreateProject: () => dispatch({ type: "open-create-project" }),
        closeCreateProject: () => dispatch({ type: "close-create-project" }),
        createProject,
        createChat,
        deleteChat,
        forkChat,
        deleteProject,
        reorderProjects,
        startProject,
        stopProject,
      }}
    >
      {children}
    </WorkspaceContext.Provider>
  );
}

export function useWorkspaceContext(): WorkspaceContextValue {
  const value = useContext(WorkspaceContext);
  if (!value) throw new Error("useWorkspaceContext must be used inside WorkspaceProvider");
  return value;
}
