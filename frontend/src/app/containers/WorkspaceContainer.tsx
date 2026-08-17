import { AppShell } from "../../ui/layout/AppShell";
import { NoChatSelected } from "../../ui/layout/NoChatSelected";
import { NewProjectDialog } from "../../ui/projects/NewProjectDialog";
import { useWorkspaceContext } from "../../state/context/WorkspaceContext";
import { useWorkspaceCommands } from "../../state/hooks/workspace/useWorkspaceCommands";
import { ChatContainer } from "./ChatContainer";
import { ProjectContainersContainer } from "./ProjectContainersContainer";
import { SettingsContainer } from "./SettingsContainer";
import { SidebarContainer } from "./SidebarContainer";

export function WorkspaceContainer() {
  const workspace = useWorkspaceContext();
  const commands = useWorkspaceCommands();

  return (
    <>
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
        ) : (
          <NoChatSelected
            hasProjects={workspace.projects.length > 0}
            onNewProject={commands.newProject}
            onNewChat={() => commands.newChatInProject(undefined)}
            onHamburger={workspace.openSidebar}
          />
        )}
      </AppShell>
      <NewProjectDialog
        state={workspace.newProject}
        onNameChange={workspace.setNewProjectName}
        onSelectTemplate={workspace.selectNewProjectTemplate}
        onSubmit={workspace.submitNewProject}
        onClose={workspace.closeNewProject}
      />
    </>
  );
}
