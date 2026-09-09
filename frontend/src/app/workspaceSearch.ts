import type { SearchPreferences } from "../port/workspaceSearch.ts";
import {
  ephemeralSearchPreferenceService,
  searchPreferenceService,
} from "../services/workspace/searchPreferenceService.ts";
import { SearchSelectionService } from "../services/workspace/searchSelectionService.ts";
import type { WorkspaceSearchSurface, WorkspaceSearchSurfaces } from "../state/context/WorkspaceSearchContext.ts";
import { createWorkspaceSearchStore } from "../state/stores/workspace/workspaceSearchStore.ts";

export function createWorkspaceSearchSurface(preferences: SearchPreferences): WorkspaceSearchSurface {
  const store = createWorkspaceSearchStore({
    filters: preferences.readFilters(),
    sort: preferences.readSort(),
  });
  return { store, selection: new SearchSelectionService(store, preferences) };
}

// Preserve page lifetime: dismissing or unmounting a surface keeps its input.
// Only the sidebar reads and writes durable preferences.
export const workspaceSearchSurfaces: WorkspaceSearchSurfaces = {
  sidebar: createWorkspaceSearchSurface(searchPreferenceService),
  palette: createWorkspaceSearchSurface(ephemeralSearchPreferenceService),
};
