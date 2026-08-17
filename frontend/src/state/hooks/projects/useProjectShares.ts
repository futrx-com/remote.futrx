import { useCallback, useState } from "preact/hooks";
import { projectApi } from "../../../api/projectApi";
import type { ContainerApp, ProjectMeta, ProjectShare } from "../../../models/project";
import type {
  ProjectDataLoadSignal,
  SharesRecord,
} from "../../projects/projectContainerRecords";
import { addShare, removeShare } from "../../projects/projectShareState";

/**
 * Public preview links for one project, alongside the container's listening
 * ports so the UI can offer a share action per discovered app.
 */
export function useProjectShares(project: ProjectMeta | null) {
  const [record, setRecord] = useState<SharesRecord>({ loading: false });

  const load = useCallback(
    async (signal?: ProjectDataLoadSignal) => {
      if (!project) {
        setRecord({ loading: false });
        return;
      }
      setRecord((current) => ({ ...current, loading: true, error: undefined }));
      try {
        // Port discovery runs `ss` inside the container, so it fails whenever
        // the project is stopped. That must not hide existing links.
        const [data, apps] = await Promise.all([
          projectApi.listShares(project.id),
          projectApi.listApps(project.id).catch(() => [] as ContainerApp[]),
        ]);
        if (signal?.cancelled) return;
        setRecord({ loading: false, data, apps });
      } catch (error) {
        if (signal?.cancelled) return;
        setRecord({ loading: false, error: (error as Error).message });
      }
    },
    [project]
  );

  const create = useCallback(
    async (port: number, ttlHours: number, label?: string): Promise<ProjectShare> => {
      if (!project) throw new Error("No project selected.");
      const created = await projectApi.createShare(project.id, { port, ttlHours, label });
      setRecord((current) => ({
        ...current,
        loading: false,
        // The url is the one-time secret; it never enters the stored list.
        data: addShare(current.data ?? [], { ...created, url: undefined }),
      }));
      return created;
    },
    [project]
  );

  const revoke = useCallback(
    async (shareId: string) => {
      if (!project) return;
      await projectApi.revokeShare(project.id, shareId);
      setRecord((current) => ({
        ...current,
        loading: false,
        data: removeShare(current.data ?? [], shareId),
      }));
    },
    [project]
  );

  return { record, load, create, revoke };
}
