import { createContext } from "preact";
import { useContext } from "preact/hooks";
import type { StoreApi } from "zustand/vanilla";
import type { WorkspaceSearchStoreActions, WorkspaceSearchStoreState } from "../../models/search.ts";
import type { SearchSelectionCommands } from "../../port/workspaceSearch.ts";

export interface WorkspaceSearchSurface {
  store: StoreApi<WorkspaceSearchStoreState & WorkspaceSearchStoreActions>;
  selection: SearchSelectionCommands;
}

export interface WorkspaceSearchSurfaces {
  sidebar: WorkspaceSearchSurface;
  palette: WorkspaceSearchSurface;
}

export const WorkspaceSearchContext = createContext<WorkspaceSearchSurfaces | null>(null);

export function useWorkspaceSearchContext(): WorkspaceSearchSurfaces {
  const value = useContext(WorkspaceSearchContext);
  if (!value) throw new Error("Workspace search requires WorkspaceSearchContext.Provider");
  return value;
}
