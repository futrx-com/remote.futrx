import { AppShell } from "../../ui/layout/AppShell";
import { NoChatSelected } from "../../ui/layout/NoChatSelected";
import { ChatSkeleton } from "../../ui/chat/ChatSkeleton";
import { CreateProjectModal } from "../../ui/projects/CreateProjectModal";
import { useWorkspaceContext } from "../../state/context/WorkspaceContext";
import { useCommandPalette } from "../../state/hooks/workspace/useCommandPalette";
import { usePaletteSearch } from "../../state/hooks/workspace/useWorkspaceSearch";
import { CommandPalette } from "../../ui/search/CommandPalette";
import { useWorkspaceCommands } from "../../state/hooks/workspace/useWorkspaceCommands";
import { ChatContainer } from "./ChatContainer";
import { ProjectContainersContainer } from "./ProjectContainersContainer";
import { SettingsContainer } from "./SettingsContainer";
import { SidebarContainer } from "./SidebarContainer";

export function WorkspaceContainer() {
  const workspace = useWorkspaceContext();
  const commands = useWorkspaceCommands();
  // The one caller of `useCommandPalette`: it binds the chord that toggles the
  // palette, and this is what renders the palette it toggles.
  const palette = useCommandPalette();
  const paletteSearch = usePaletteSearch();
  // Two moments where there is no chat to render but one is still coming: the
  // snapshot has not landed, or it has and the initial-chat effect has not run
  // its pick yet. Both would otherwise flash the "Create your first project"
  // pitch at someone who already has projects.
  const chatPending =
    !workspace.loaded || (!workspace.activeChat && workspace.chats.length > 0);

  return (
    <AppShell sidebar={<SidebarContainer />}>
      {workspace.ui.view === "settings" ? (
        <SettingsContainer
          onBack={workspace.showChat}
          onHamburger={workspace.openSidebar}
        />
      ) : workspace.ui.view === "project-containers" ? (
        <ProjectContainersContainer
          projects={workspace.projects}
          selectedProjectId={workspace.ui.containerProjectId}
          onBack={workspace.showChat}
          onHamburger={workspace.openSidebar}
          onDeleteProject={workspace.deleteProject}
        />
      ) : workspace.activeChat ? (
        <ChatContainer
          key={workspace.activeChat.id}
          chat={workspace.activeChat}
          projects={workspace.projects}
          onHamburger={workspace.openSidebar}
        />
      ) : chatPending ? (
        <ChatSkeleton onHamburger={workspace.openSidebar} />
      ) : (
        <NoChatSelected
          hasProjects={workspace.projects.length > 0}
          onNewProject={commands.newProject}
          onNewChat={() => commands.newChatInProject(undefined)}
          onHamburger={workspace.openSidebar}
        />
      )}
      <CreateProjectModal
        open={workspace.ui.createProjectOpen}
        projects={workspace.projects}
        onClose={workspace.closeCreateProject}
        onCreate={workspace.createProject}
      />
      <CommandPalette
        search={paletteSearch}
        open={palette.open}
        onClose={palette.close}
        onSelectChat={workspace.selectChat}
      />
    </AppShell>
  );
}
