import { useMemo } from "preact/hooks";
import { useStore } from "zustand";
import type { StoreApi } from "zustand/vanilla";
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
  WorkspaceSearchStoreState,
} from "../../../models/search.ts";
import { searchFilterService } from "../../../services/workspace/searchFilterService.ts";
import { workspaceSearchService } from "../../../services/workspace/workspaceSearchService.ts";
import {
  paletteSearchStore,
  sidebarSearchStore,
} from "../../stores/workspace/workspaceSearchStore.ts";
import { useWorkspaceContext } from "../../context/WorkspaceContext";

/** A handle on one surface's selection. Named here because `stores/` declares
 *  no types and a store handle is not a data shape. */
export type WorkspaceSearchStore = StoreApi<
  WorkspaceSearchStoreState & WorkspaceSearchStoreActions
>;

// Each surface takes the commands straight from the store's contract, so their
// signatures are stated once, in `models/search.ts`, and a change to one of
// them reaches the components that call it.

/** The keyword box: the text, and how to change it. */
export interface QueryControl extends Pick<WorkspaceSearchStoreActions, "setQuery"> {
  query: string;
}

/** The filter menu and the chips: the selection, and every way to change it. */
export interface FilterControl extends Pick<
  WorkspaceSearchStoreActions,
  | "setSort"
  | "toggleFacetValue"
  | "setFacetValues"
  | "clearFacet"
  | "setDateFilter"
  | "clearDate"
  | "resetFilters"
  | "retainCounts"
> {
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
    Pick<WorkspaceSearchStoreActions, "clearAll"> {}

// Closes over nothing, so it is built once rather than per render. Forwarded
// rather than handed over as a method reference, which would depend on the
// service never coming to need `this`.
const describeMatch = (hit: SearchHit) => workspaceSearchService.describeMatch(hit);

/** The sidebar's search: its selection is remembered across reloads. */
export function useSidebarSearch(): WorkspaceSearch {
  const workspace = useWorkspaceContext();
  return useWorkspaceSearch(sidebarSearchStore, workspace.chats, workspace.projects);
}

/** The palette's search, independent of the sidebar's and saved nowhere. */
export function usePaletteSearch(): WorkspaceSearch {
  const workspace = useWorkspaceContext();
  return useWorkspaceSearch(paletteSearchStore, workspace.chats, workspace.projects);
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
  store: WorkspaceSearchStore,
  chats: readonly ChatMeta[],
  projects: readonly ProjectMeta[],
): WorkspaceSearch {
  const query = useStore(store, (state) => state.query);
  const filters = useStore(store, (state) => state.filters);
  const sort = useStore(store, (state) => state.sort);
  const countsEnabled = useStore(store, (state) => state.countsRetained > 0);

  // The commands, read the same way the state above is. Each is an identity
  // the store creates once, so a selector per command costs nothing and none
  // of them can trigger a render -- which an object of them all would, on
  // every notification, being a new object each time it was selected.
  const setQuery = useStore(store, (state) => state.setQuery);
  const setSort = useStore(store, (state) => state.setSort);
  const toggleFacetValue = useStore(store, (state) => state.toggleFacetValue);
  const setFacetValues = useStore(store, (state) => state.setFacetValues);
  const clearFacet = useStore(store, (state) => state.clearFacet);
  const setDateFilter = useStore(store, (state) => state.setDateFilter);
  const clearDate = useStore(store, (state) => state.clearDate);
  const resetFilters = useStore(store, (state) => state.resetFilters);
  const clearAll = useStore(store, (state) => state.clearAll);
  const retainCounts = useStore(store, (state) => state.retainCounts);

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
    () => workspaceSearchService.facetViews(docs, filters, outcome),
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
