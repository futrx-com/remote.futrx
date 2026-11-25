import { useCallback, useMemo } from "preact/hooks";
import type { ChatSearchHit } from "../../api/chat/chatTranscriptApi";
import { chatJumpRequestService } from "../../services/chat/chatJumpRequestService.ts";
import { useMessageSearch } from "../../state/hooks/chat/useMessageSearch";
import { Sidebar } from "../../ui/sidebar/Sidebar";
import { useAuthContext } from "../../state/context/AuthContext";
import { useWorkspaceContext } from "../../state/context/WorkspaceContext";
import { useSidebarState } from "../../state/hooks/workspace/useSidebarState";
import { useOpenCommandPalette } from "../../state/hooks/workspace/useCommandPalette";
import { useSidebarSearch } from "../../state/hooks/workspace/useWorkspaceSearch";
import { useWorkspaceCommands } from "../../state/hooks/workspace/useWorkspaceCommands";
import { workspaceSidebarService } from "../../services/workspace/workspaceSidebarService.ts";
import { useAccountSignOut } from "../../state/hooks/auth/useAccountSignOut";

export function SidebarContainer({ updateTag }: { updateTag: string | null }) {
  const { auth } = useAuthContext();
  const workspace = useWorkspaceContext();
  const sidebar = useSidebarState(
    workspace.ui.sidebarOpen,
    workspace.closeSidebar,
    workspace.projects,
    workspace.chats
  );
  const commands = useWorkspaceCommands();
  const signOut = useAccountSignOut(auth.email || auth.adminEmail);
  const search = useSidebarSearch();
  const openPalette = useOpenCommandPalette();
  const messageSearch = useMessageSearch(search.query, search.selection);
  const titles = useMemo(
    () => new Map(workspace.chats.map((chat) => [chat.id, chat.title])),
    [workspace.chats],
  );
  const chatTitle = useCallback((chatId: string) => titles.get(chatId) || "Untitled chat", [titles]);
  const { selectChat } = workspace;
  const openMessage = useCallback((hit: ChatSearchHit) => {
    chatJumpRequestService.request({ chatId: hit.chatId, query: search.query.trim(), hit });
    selectChat(hit.chatId);
  }, [search.query, selectChat]);
  const model = useMemo(
    () => workspaceSidebarService.model(workspace.chats, workspace.projects),
    [workspace.chats, workspace.projects]
  );

  return (
    <Sidebar
      open={workspace.ui.sidebarOpen}
      model={model}
      search={search}
      loading={!workspace.loaded}
      collapsed={sidebar.collapsed}
      sidebarCollapsed={sidebar.sidebarCollapsed}
      activeChatId={workspace.ui.activeChatId}
      account={{
        email: auth.email,
        authenticated: auth.authenticated,
      }}
      onClose={workspace.closeSidebar}
      onOpenPalette={openPalette}
      onToggleSidebar={sidebar.toggleSidebarCollapsed}
      onNewProject={commands.newProject}
      onNewChatInProject={commands.newChatInProject}
      onToggleProject={sidebar.toggleCollapsed}
      onSelectChat={workspace.selectChat}
      messageSearch={messageSearch}
      chatTitle={chatTitle}
      onOpenMessage={openMessage}
      onDeleteChat={commands.deleteChat}
      onToggleChatUnread={commands.toggleChatUnread}
      onForkChat={commands.forkChat}
      onRenameChat={commands.renameChat}
      onReorderProjects={commands.reorderProjects}
      onOpenProjectContainers={workspace.showProjectContainers}
      updateTag={updateTag}
      onOpenUpdates={() => workspace.showSettings("updates")}
      onOpenSettings={() => workspace.showSettings()}
      onSignOut={signOut}
    />
  );
}
