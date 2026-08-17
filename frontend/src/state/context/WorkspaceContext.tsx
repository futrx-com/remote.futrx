import type { ComponentChildren } from "preact";
import { createContext } from "preact";
import { useContext, useEffect, useReducer } from "preact/hooks";
import type { ChatMeta, CreateChatInput } from "../../models/chat";
import type { ProjectMeta } from "../../models/project";
import { chatApi } from "../../api/chatApi";
import { projectApi } from "../../api/projectApi";
import { templateApi } from "../../api/templateApi";
import { useWorkspaceData } from "../hooks/workspace/useWorkspaceData";
import { useUserSettingsContext } from "./UserSettingsContext";
import {
  workspaceUiState,
  type WorkspaceUiState,
} from "../workspace/workspaceUiState";
import { workspaceSidebarState } from "../workspace/workspaceSidebarState";
import {
  newProjectState,
  type NewProjectState,
} from "../projects/newProjectState";

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
  newProject: NewProjectState;
  openNewProject: () => void;
  closeNewProject: () => void;
  setNewProjectName: (name: string) => void;
  selectNewProjectTemplate: (template: string) => void;
  submitNewProject: () => Promise<void>;
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
  const { settings } = useUserSettingsContext();
  const [ui, dispatch] = useReducer(workspaceUiState.reduce, workspaceUiState.createInitial());
  const [newProject, dispatchNewProject] = useReducer(
    newProjectState.reduce,
    newProjectState.createInitial()
  );
  const activeChat = workspaceSidebarState.activeChat(data.chats, ui.activeChatId);

  useEffect(() => {
    const chatId = workspaceSidebarState.initialChatId(enabled, ui.activeChatId, data.chats);
    if (chatId) dispatch({ type: "select-chat", chatId });
  }, [data.chats, enabled, ui.activeChatId]);

  useEffect(() => {
    if (workspaceSidebarState.isActiveChatMissing(data.chats, ui.activeChatId)) {
      dispatch({ type: "select-chat", chatId: null });
    }
  }, [data.chats, ui.activeChatId]);

  // The template catalog is fetched once the dialog is first opened, not on
  // mount: it is a static list only this dialog needs.
  function openNewProject() {
    dispatchNewProject({ type: "open" });
    if (newProject.templates.length > 0 || newProject.templatesLoading) return;
    dispatchNewProject({ type: "templates-loading" });
    templateApi
      .list()
      .then((templates) => dispatchNewProject({ type: "templates-loaded", templates }))
      .catch((error: Error) =>
        dispatchNewProject({ type: "templates-failed", error: error.message })
      );
  }

  async function submitNewProject(): Promise<void> {
    const name = newProjectState.submittedName(newProject);
    if (!name) return;
    dispatchNewProject({ type: "submit" });
    try {
      await projectApi.create(name, newProject.template);
      dispatchNewProject({ type: "close" });
    } catch (error) {
      dispatchNewProject({ type: "submit-failed", error: (error as Error).message });
    }
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
  }

  async function reorderProjects(projectIds: string[]) {
    await projectApi.reorder(projectIds);
  }

  async function startProject(projectId: string) {
    await projectApi.start(projectId);
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
        newProject,
        openNewProject,
        closeNewProject: () => dispatchNewProject({ type: "close" }),
        setNewProjectName: (name) => dispatchNewProject({ type: "set-name", name }),
        selectNewProjectTemplate: (template) =>
          dispatchNewProject({ type: "select-template", template }),
        submitNewProject,
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
