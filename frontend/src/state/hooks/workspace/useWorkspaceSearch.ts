import { useMemo } from "preact/hooks";
import { useStore } from "zustand";
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
  return useWorkspaceSearch(sidebar, workspace.chats, workspace.projects);
}

/** The palette's search, independent of the sidebar's and saved nowhere. */
export function usePaletteSearch(): WorkspaceSearch {
  const workspace = useWorkspaceContext();
  const { palette } = useWorkspaceSearchContext();
  return useWorkspaceSearch(palette, workspace.chats, workspace.projects);
}

/**
 * Reads one search store and derives what the surfaces render from it.
 *
 * The selection lives in the store, so it survives the surface unmounting and
 * two components reading the same store agree. The index and the results do
 * not: they are a function of the selection and of the chats the feed is
 * pushing, cached per render pass here rather than duplicated into state that
 * could fall behind either input.
 *
 * The index is rebuilt only when chats or projects change, so keystrokes pay
 * for comparison alone. Facet counts are computed only while a filter menu is
 * open, since nothing else displays them.
 */
export function useWorkspaceSearch(
  { store, selection }: WorkspaceSearchSurface,
  chats: readonly ChatMeta[],
  projects: readonly ProjectMeta[],
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
  } = selection;

  const docs = useMemo(
    () => workspaceSearchService.buildIndex(chats, projects),
    [chats, projects],
  );

  // `now` is pinned per render pass rather than read inside the search, so a
  // date-bounded result set can't shift underneath a single render.
  const outcome = useMemo(
    () =>
      workspaceSearchService.run(docs, filters, query, sort, Date.now(), {
        withCounts: countsEnabled,
      }),
    [docs, filters, query, sort, countsEnabled],
  );

  const facetViews = useMemo(
    () => searchFacetService.facetViews(docs, filters, outcome),
    [docs, filters, outcome],
  );

  const dateView: DateFilterView = {
    active: searchFilterService.isDateActive(filters.date),
    label: searchFilterService.describeDate(filters.date),
  };

  const activeFilterCount = searchFilterService.countActive(filters);

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
    isSearching: query.trim().length > 0 || activeFilterCount > 0,
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
