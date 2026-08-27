import { useMemo } from "preact/hooks";
import { Sidebar } from "../../ui/sidebar/Sidebar";
import { useAuthContext } from "../../state/context/AuthContext";
import { useWorkspaceContext } from "../../state/context/WorkspaceContext";
import { useSidebarState } from "../../state/hooks/workspace/useSidebarState";
import { useWorkspaceCommands } from "../../state/hooks/workspace/useWorkspaceCommands";
import { workspaceSidebarState } from "../../state/workspace/workspaceSidebarState";
import { useAccountSignOut } from "../../state/hooks/auth/useAccountSignOut";

export function SidebarContainer() {
  const { auth } = useAuthContext();
  const workspace = useWorkspaceContext();
  const sidebar = useSidebarState(
    workspace.ui.sidebarOpen,
    workspace.closeSidebar,
    workspace.projects,
    workspace.chats
  );
  const commands = useWorkspaceCommands();
  const signOut = useAccountSignOut();
  const model = useMemo(
    () => workspaceSidebarState.model(workspace.chats, workspace.projects, sidebar.query),
    [workspace.chats, workspace.projects, sidebar.query]
  );

  return (
    <Sidebar
      open={workspace.ui.sidebarOpen}
      model={model}
      query={sidebar.query}
      collapsed={sidebar.collapsed}
      sidebarCollapsed={sidebar.sidebarCollapsed}
      activeChatId={workspace.ui.activeChatId}
      account={{
        email: auth.email,
        authenticated: auth.authenticated,
      }}
      onClose={workspace.closeSidebar}
      onQueryChange={sidebar.setQuery}
      onClearQuery={() => sidebar.setQuery("")}
      onToggleSidebar={sidebar.toggleSidebarCollapsed}
      onNewProject={commands.newProject}
      onNewChatInProject={commands.newChatInProject}
      onToggleProject={sidebar.toggleCollapsed}
      onSelectChat={workspace.selectChat}
      onDeleteChat={commands.deleteChat}
      onToggleChatUnread={commands.toggleChatUnread}
      onForkChat={commands.forkChat}
      onReorderProjects={commands.reorderProjects}
      onOpenProjectContainers={workspace.showProjectContainers}
      onOpenSettings={workspace.showSettings}
      onSignOut={signOut}
    />
  );
}
