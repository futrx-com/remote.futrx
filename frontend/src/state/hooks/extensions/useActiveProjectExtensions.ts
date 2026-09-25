import { useStore } from "zustand";
import { useEffect } from "preact/hooks";
import { extensionStore } from "../../stores/extensions/extensionStore";

export function useActiveProjectExtensions(
  projectId: string | null | undefined,
): void {
  const setActiveProject = useStore(
    extensionStore,
    (state) => state.setActiveProject,
  );
  useEffect(() => {
    setActiveProject(projectId ?? null);
  }, [projectId, setActiveProject]);
}
