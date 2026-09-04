import { createStore } from "zustand/vanilla";
import type {
  SearchFilters,
  SearchPreferences,
  WorkspaceSearchStoreActions,
  WorkspaceSearchStoreState,
} from "../../../models/search";
import { searchFilterService } from "../../../services/workspace/searchFilterService.ts";
import {
  ephemeralSearchPreferenceService,
  searchPreferenceService,
} from "../../../services/workspace/searchPreferenceService.ts";

/**
 * One surface's search selection, held outside the component tree.
 *
 * Only the selection lives here. The index and the ranked results are derived
 * from it and from the chats the workspace feed is pushing, so they are
 * computed where both are in hand -- in `useWorkspaceSearch` -- rather than
 * mirrored into a second store that would have to be kept in step with the
 * first.
 *
 * `preferences` is injected rather than imported so the two instances below can
 * differ in exactly one thing: whether the selection outlives the session. It
 * is also what keeps this module free of a fixed storage key, which is what
 * lets a test drive it with a hand-held boundary.
 */
export function createWorkspaceSearchStore(preferences: SearchPreferences) {
  return createStore<WorkspaceSearchStoreState & WorkspaceSearchStoreActions>()(
    (set, get) => {
      /**
       * The one way the selection changes: publish once, then save.
       *
       * Publishing once is what keeps a subscriber from seeing a half-applied
       * selection -- clearing the search is a single change, not a cleared
       * keyword followed by cleared filters. Saving here rather than from an
       * effect is what keeps hydration from writing straight back: the initial
       * selection comes from the initializer below, which never reaches this.
       */
      function commitSelection(filters: SearchFilters, query = get().query): void {
        set({ query, filters });
        preferences.writeFilters(filters);
      }

      /** What every filter action does; the rule itself belongs to the service. */
      function changeFilters(change: (filters: SearchFilters) => SearchFilters): void {
        commitSelection(change(get().filters));
      }

      return {
        query: "",
        filters: preferences.readFilters(),
        sort: preferences.readSort(),
        countsRetained: 0,

        setQuery: (query) => set({ query }),

        setSort: (sort) => {
          set({ sort });
          preferences.writeSort(sort);
        },

        toggleFacetValue: (facetId, value) => changeFilters(
          (filters) => searchFilterService.toggleFacetValue(filters, facetId, value),
        ),

        setFacetValues: (facetId, values) => changeFilters(
          (filters) => searchFilterService.withFacetValues(filters, facetId, values),
        ),

        clearFacet: (facetId) => changeFilters(
          (filters) => searchFilterService.clearFacet(filters, facetId),
        ),

        setDateFilter: (date) => changeFilters(
          (filters) => searchFilterService.withDate(filters, date),
        ),

        clearDate: () => changeFilters(
          (filters) => searchFilterService.withDate(
            filters,
            searchFilterService.clearedDate(filters.date),
          ),
        ),

        resetFilters: () => commitSelection(searchFilterService.defaults()),

        clearAll: () => commitSelection(searchFilterService.defaults(), ""),

        retainCounts: () => {
          set((state) => ({ countsRetained: state.countsRetained + 1 }));
          let released = false;
          return () => {
            // Single-use, so a menu that releases twice cannot drop the count
            // below the number of menus still open.
            if (released) return;
            released = true;
            set((state) => ({ countsRetained: state.countsRetained - 1 }));
          };
        },
      };
    },
  );
}

// A search state per surface. They were one shared state, which meant narrowing
// the palette to a project silently re-scoped the sidebar behind it -- a filter
// you never set, on a list you were not looking at.
//
// Both read the same chats and projects, so results agree; only the selection is
// separate. The sidebar's is a place you set up and come back to, so it is
// remembered across reloads; the palette is a scratch surface you open, use and
// dismiss, so it starts from the defaults every time and saves none of it.
export const sidebarSearchStore = createWorkspaceSearchStore(searchPreferenceService);

export const paletteSearchStore = createWorkspaceSearchStore(ephemeralSearchPreferenceService);
