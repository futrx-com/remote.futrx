import { useCallback, useState } from "preact/hooks";
import { projectApi } from "../../../api/projectApi";
import type { ProjectMeta } from "../../../models/project";
import type {
  ProjectContainerRecord,
  ProjectDataLoadSignal,
} from "../../projects/projectContainerRecords";

export function useProjectContainerInfo(project: ProjectMeta | null) {
  const [record, setRecord] = useState<ProjectContainerRecord>({ loading: false });

  const load = useCallback(
    async (signal?: ProjectDataLoadSignal) => {
      if (!project) {
        setRecord({ loading: false });
        return;
      }
      setRecord((current) => ({ ...current, loading: true, error: undefined }));
      try {
        const data = await projectApi.fetchContainerInfo(project.id);
        if (signal?.cancelled) return;
        setRecord({ loading: false, data, refreshedAt: Date.now() });
      } catch (error) {
        if (signal?.cancelled) return;
        setRecord({
          loading: false,
          error: (error as Error).message,
          refreshedAt: Date.now(),
        });
      }
    },
    [project]
  );

  const repairNetwork = useCallback(async () => {
    if (!project) return;
    const data = await projectApi.repairNetwork(project.id);
    setRecord({ loading: false, data, refreshedAt: Date.now() });
  }, [project]);

  // force skips the aggregate host-memory guard; the server still rejects it
  // for a non-admin caller.
  const start = useCallback(async (force = false) => {
    if (!project) return;
    await projectApi.start(project.id, force);
    await load();
  }, [project, load]);

  const stop = useCallback(async () => {
    if (!project) return;
    await projectApi.stop(project.id);
    await load();
  }, [project, load]);

  const restart = useCallback(async () => {
    if (!project) return;
    await projectApi.restart(project.id);
    await load();
  }, [project, load]);

  return { record, load, repairNetwork, start, stop, restart };
}
