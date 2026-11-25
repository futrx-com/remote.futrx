import { useMemo } from "preact/hooks";
import { useStore } from "zustand";
import type { WorkspaceSearchSelection } from "../../../api/chat/workspaceSearchApi.ts";
import type { ChatMeta } from "../../../models/chat.ts";
import type { ProjectMeta } from "../../../models/project.ts";
import type {
  DateFilterView,
  FacetView,
  SearchFilters,
  SearchHit,
  SearchOutcome,
  SortId,
  WorkspaceSearchStoreActions,
} from "../../../models/search.ts";
import { searchFilterService } from "../../../services/workspace/searchFilterService.ts";
import { workspaceSearchService } from "../../../services/workspace/workspaceSearchService.ts";
import { searchFacetService } from "../../../services/workspace/searchFacetService.ts";
import type { SearchSelectionCommands } from "../../../port/workspaceSearch.ts";
import { useWorkspaceSearchContext } from "../../context/WorkspaceSearchContext.ts";
import type { WorkspaceSearchSurface } from "../../context/WorkspaceSearchContext.ts";
import { useWorkspaceContext } from "../../context/WorkspaceContext";
import { useWorkspaceSearchResults } from "./useWorkspaceSearchResults.ts";

/** The keyword box: the text, and how to change it. */
export interface QueryControl extends Pick<WorkspaceSearchStoreActions, "setQuery"> {
  query: string;
}

/** The filter menu and the chips: the selection, and every way to change it. */
export interface FilterControl extends Pick<
  SearchSelectionCommands,
  | "setSort"
  | "toggleFacetValue"
  | "setFacetValues"
  | "clearFacet"
  | "setDateFilter"
  | "clearDate"
  | "resetFilters"
>, Pick<WorkspaceSearchStoreActions, "retainCounts"> {
  filters: SearchFilters;
  facetViews: FacetView[];
  dateView: DateFilterView;
  activeFilterCount: number;
  hasActiveFilters: boolean;
  sort: SortId;
}

/** What anything that renders results needs, and nothing more. */
export interface ResultsView {
  outcome: SearchOutcome;
  loading: boolean;
  error: string | null;
  selection: WorkspaceSearchSelection;
  /** True when a keyword or any filter is narrowing the list. */
  isSearching: boolean;
  /** Why a hit is in the list, when its title alone doesn't show it. */
  describeMatch: (hit: SearchHit) => string | null;
}

/**
 * The whole search surface. Components take the slice they use: the chips and
 * the filter menu take `FilterControl` and cannot reach the query or the
 * results; the palette takes all of it because it genuinely is all of it.
 */
export interface WorkspaceSearch
  extends QueryControl,
    FilterControl,
    ResultsView,
    Pick<SearchSelectionCommands, "clearAll"> {}

// Closes over nothing, so it is built once rather than per render. Forwarded
// rather than handed over as a method reference, which would depend on the
// service never coming to need `this`.
const describeMatch = (hit: SearchHit) => workspaceSearchService.describeMatch(hit);

/** The sidebar's search: its selection is remembered across reloads. */
export function useSidebarSearch(): WorkspaceSearch {
  const workspace = useWorkspaceContext();
  const { sidebar } = useWorkspaceSearchContext();
  return useWorkspaceSearch(sidebar, workspace.chats, workspace.projects, workspace.loaded, false);
}

/** The palette's search, independent of the sidebar's and saved nowhere. */
export function usePaletteSearch(open: boolean): WorkspaceSearch {
  const workspace = useWorkspaceContext();
  const { palette } = useWorkspaceSearchContext();
  return useWorkspaceSearch(palette, workspace.chats, workspace.projects, workspace.loaded && open, true);
}

/** Keep selection in the store and fetch matching chats and facet counts. */
function useWorkspaceSearch(
  { store, selection: commands }: WorkspaceSearchSurface,
  chats: readonly ChatMeta[],
  projects: readonly ProjectMeta[],
  enabled: boolean,
  includeRecent: boolean,
): WorkspaceSearch {
  const query = useStore(store, (state) => state.query);
  const filters = useStore(store, (state) => state.filters);
  const sort = useStore(store, (state) => state.sort);
  const countsEnabled = useStore(store, (state) => state.countsRetained > 0);

  const setQuery = useStore(store, (state) => state.setQuery);
  const retainCounts = useStore(store, (state) => state.retainCounts);
  const {
    setSort, toggleFacetValue, setFacetValues, clearFacet,
    setDateFilter, clearDate, resetFilters, clearAll,
  } = commands;

  const selection = useMemo<WorkspaceSearchSelection>(() => ({
    facets: filters.facets,
    dateField: filters.date.field,
    ...searchFilterService.resolveDateRange(filters.date, Date.now()),
  }), [filters]);
  const activeFilterCount = searchFilterService.countActive(filters);
  const isSearching = query.trim().length > 0 || activeFilterCount > 0;
  const fetchResults = enabled && (includeRecent || isSearching || countsEnabled);
  const request = useMemo(
    () => ({ ...selection, query, sort, withCounts: countsEnabled }),
    [selection, query, sort, countsEnabled],
  );
  const { outcome, loading, error } = useWorkspaceSearchResults(request, fetchResults, chats, projects);
  const facetViews = useMemo(
    () => searchFacetService.viewsFromValues(outcome.facetValues, filters.facets, outcome.counts, projects),
    [outcome, filters.facets, projects],
  );

  const dateView: DateFilterView = {
    active: searchFilterService.isDateActive(filters.date),
    label: searchFilterService.describeDate(filters.date),
  };

  return {
    query,
    setQuery,
    filters,
    sort,
    setSort,
    outcome,
    facetViews,
    dateView,
    activeFilterCount,
    hasActiveFilters: activeFilterCount > 0,
    isSearching,
    loading,
    error,
    selection,
    describeMatch,
    toggleFacetValue,
    setFacetValues,
    clearFacet,
    setDateFilter,
    clearDate,
    resetFilters,
    clearAll,
    retainCounts,
  };
}
