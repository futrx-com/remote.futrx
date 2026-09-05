import { useCallback, useEffect, useMemo, useState } from "preact/hooks";
import type { ProjectMeta } from "../../../models/project";
import { useProjectAccess } from "./useProjectAccess";
import { useProjectContainerInfo } from "./useProjectContainerInfo";
import { useProjectSecrets } from "./useProjectSecrets";
import { useProjectShares } from "./useProjectShares";

export function useProjectContainersController(
  projects: ProjectMeta[],
  selectedProjectId: string | null
) {
  const selectedProject = useMemo(
    () => projects.find((project) => project.id === selectedProjectId) ?? null,
    [projects, selectedProjectId]
  );
  const info = useProjectContainerInfo(selectedProject);
  const secrets = useProjectSecrets(selectedProject);
  const access = useProjectAccess(selectedProject);
  const shares = useProjectShares(selectedProject);
  const [refreshing, setRefreshing] = useState(false);

  const refresh = useCallback(async () => {
    if (!selectedProject) return;
    setRefreshing(true);
    try {
      await Promise.all([info.load(), secrets.load(), access.load(), shares.load()]);
    } finally {
      setRefreshing(false);
    }
  }, [selectedProject, info.load, secrets.load, access.load, shares.load]);

  useEffect(() => {
    const signal = { cancelled: false };
    void info.load(signal);
    void secrets.load(signal);
    void access.load(signal);
    void shares.load(signal);
    return () => {
      signal.cancelled = true;
    };
  }, [info.load, secrets.load, access.load, shares.load]);

  return {
    selectedProject,
    info,
    secrets,
    access,
    shares,
    refreshing,
    refresh,
  };
}
