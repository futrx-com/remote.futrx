import { useCallback, useState } from "preact/hooks";
import {
  ProjectContainersPage,
  type ProjectSettingsTab,
} from "../../ui/projects/ProjectContainersPage";
import type { ProjectMeta } from "../../models/project";
import { useProjectContainersController } from "../../state/hooks/projects/useProjectContainersController";
import { useProjectApplications } from "../../state/hooks/applications/useApplications";
import { useAuthContext } from "../../state/context/AuthContext";
import { useServerInfo } from "../../state/hooks/server/useServerInfo";

export function ProjectContainersContainer({
  projects,
  selectedProjectId,
  onBack,
  onHamburger,
  onDeleteProject,
}: {
  projects: ProjectMeta[];
  selectedProjectId: string | null;
  onBack: () => void;
  onHamburger: () => void;
  onDeleteProject: (projectId: string) => Promise<void>;
}) {
  const { auth } = useAuthContext();
  const controller = useProjectContainersController(projects, selectedProjectId);
  const { selectedProject, info, secrets, access } = controller;
  const [activeTab, setActiveTab] = useState<ProjectSettingsTab>("info");
  const serverInfo = useServerInfo(activeTab === "settings");
  const applications = useProjectApplications(
    selectedProject,
    activeTab === "applications"
  );

  const deleteSelectedProject = useCallback(async () => {
    if (!selectedProject) return;
    await onDeleteProject(selectedProject.id);
    onBack();
  }, [selectedProject, onBack, onDeleteProject]);

  return (
    <ProjectContainersPage
      activeTab={activeTab}
      project={selectedProject}
      infoRecord={info.record}
      secretsRecord={secrets.record}
      accessRecord={access.record}
      refreshing={controller.refreshing}
      isAdmin={auth.isAdmin}
      serverMemoryTotalBytes={serverInfo.info?.memory.totalBytes}
      serverMemoryLoading={serverInfo.loading}
      onRefresh={() => void controller.refresh()}
      onBack={onBack}
      onHamburger={onHamburger}
      onTabChange={setActiveTab}
      onSaveSecret={secrets.save}
      onDeleteSecret={secrets.remove}
      applications={applications}
      onAddMember={access.add}
      onRemoveMember={access.remove}
      onRepairNetwork={info.repairNetwork}
      onSetResourceLimits={info.setLimits}
      onStartProject={info.start}
      onStopProject={info.stop}
      onRestartProject={info.restart}
      onDeleteProject={deleteSelectedProject}
    />
  );
}
