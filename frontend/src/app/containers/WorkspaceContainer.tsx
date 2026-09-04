import { AppShell } from "../../ui/layout/AppShell";
import { NoChatSelected } from "../../ui/layout/NoChatSelected";
import { ChatSkeleton } from "../../ui/chat/ChatSkeleton";
import { CreateProjectModal } from "../../ui/projects/CreateProjectModal";
import { useWorkspaceContext } from "../../state/context/WorkspaceContext";
import { useWorkspaceCommands } from "../../state/hooks/workspace/useWorkspaceCommands";
import { ChatContainer } from "./ChatContainer";
import { useActiveProjectExtensions } from "../../state/hooks/extensions/useActiveProjectExtensions";
import { ProjectContainersContainer } from "./ProjectContainersContainer";
import { SettingsContainer } from "./SettingsContainer";
import { SidebarContainer } from "./SidebarContainer";

export function WorkspaceContainer() {
  const workspace = useWorkspaceContext();
  const commands = useWorkspaceCommands();
  // Two moments where there is no chat to render but one is still coming: the
  // snapshot has not landed, or it has and the initial-chat effect has not run
  // its pick yet. Both would otherwise flash the "Create your first project"
  // pitch at someone who already has projects.
  const chatPending =
    !workspace.loaded || (!workspace.activeChat && workspace.chats.length > 0);

  // The project the user is in right now: the one whose settings are open, or
  // the one owning the chat they are reading. Project-installed extensions
  // follow this; with neither, no project is active and only global ones show.
  useActiveProjectExtensions(
    workspace.ui.view === "project-containers"
      ? workspace.ui.containerProjectId
      : workspace.activeChat?.projectId
  );

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
    </AppShell>
  );
}
