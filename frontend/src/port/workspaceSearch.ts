import type {
  DateFilter, FacetId, SearchFilters, SortId,
  WorkspaceSearchStoreActions, WorkspaceSearchStoreState,
} from "../models/search.ts";

/** How a search surface loads and saves its selection. */
export interface SearchPreferences {
  readFilters(): SearchFilters;
  writeFilters(filters: SearchFilters): void;
  readSort(): SortId;
  writeSort(sort: SortId): void;
}

/** The selection state and atomic mutations required by selection commands. */
export interface SearchSelectionStore {
  getState(): Pick<WorkspaceSearchStoreState, "query" | "filters">
    & Pick<WorkspaceSearchStoreActions, "replaceSelection" | "setSort">;
}

/** User intents that change a surface's remembered selection. */
export interface SearchSelectionCommands {
  setSort: (sort: SortId) => void;
  toggleFacetValue: (facetId: FacetId, value: string) => void;
  setFacetValues: (facetId: FacetId, values: string[]) => void;
  clearFacet: (facetId: FacetId) => void;
  setDateFilter: (date: DateFilter) => void;
  clearDate: () => void;
  resetFilters: () => void;
  clearAll: () => void;
}
