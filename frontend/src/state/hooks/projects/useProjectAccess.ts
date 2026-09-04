import { useCallback, useState } from "preact/hooks";
import { projectApi } from "../../../api/projectApi";
import type {
  AccessRecord,
  ProjectDataLoadSignal,
  ProjectMeta,
} from "../../../models/project";

export function useProjectAccess(project: ProjectMeta | null) {
  const [record, setRecord] = useState<AccessRecord>({ loading: false });

  const load = useCallback(
    async (signal?: ProjectDataLoadSignal) => {
      if (!project) {
        setRecord({ loading: false });
        return;
      }
      setRecord((current) => ({ ...current, loading: true, error: undefined }));
      try {
        const data = await projectApi.listAccess(project.id);
        if (signal?.cancelled) return;
        setRecord({ loading: false, data });
      } catch (error) {
        if (signal?.cancelled) return;
        setRecord({ loading: false, error: (error as Error).message });
      }
    },
    [project]
  );

  const add = useCallback(
    async (email: string) => {
      if (!project) return;
      const { email: added } = await projectApi.addAccess(project.id, email);
      setRecord((current) => {
        const next = current.data ? [...current.data] : [];
        if (!next.includes(added)) next.push(added);
        next.sort();
        return { loading: false, data: next };
      });
    },
    [project]
  );

  const remove = useCallback(
    async (email: string) => {
      if (!project) return;
      await projectApi.removeAccess(project.id, email);
      setRecord((current) => ({
        loading: false,
        data: current.data?.filter((member) => member !== email) ?? [],
      }));
    },
    [project]
  );

  return { record, load, add, remove };
}
