import { createStore } from "zustand/vanilla";
import type { FileOpenerEntry, FileOpenerStoreState } from "../../../models/files.ts";

function createFileOpenerStore() {
  // Keep the live map private: opener callbacks may register or dispose editors
  // while resolution is iterating, and registration order must remain stable.
  const openers = new Map<string, FileOpenerEntry>();
  return createStore<FileOpenerStoreState>()((set) => {
    const notify = () => set((state) => ({ revision: state.revision + 1 }));
    return {
      revision: 0,
      register(applicationId, projectIds, open) {
        const entry = { open, projectIds };
        openers.set(applicationId, entry);
        notify();
        return () => {
          if (openers.get(applicationId) === entry) {
            openers.delete(applicationId);
            notify();
          }
        };
      },
      setProjects(applicationId, projectIds) {
        const entry = openers.get(applicationId);
        if (entry && (entry.projectIds.length !== projectIds.length ||
          entry.projectIds.some((id) => !projectIds.includes(id)))) {
          entry.projectIds = projectIds;
          notify();
        }
      },
      remove(applicationId) {
        if (openers.delete(applicationId)) notify();
      },
      canOpen(projectId) {
        return Boolean(projectId && [...openers.values()].some((entry) => entry.projectIds.includes(projectId)));
      },
      *forProject(projectId) {
        if (!projectId) return;
        for (const entry of openers.values()) {
          if (entry.projectIds.includes(projectId)) yield (request) => entry.open(request);
        }
      },
    };
  });
}

export const fileOpenerStore = createFileOpenerStore();
