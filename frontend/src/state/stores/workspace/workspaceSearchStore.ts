import { createStore } from "zustand/vanilla";
import type {
  WorkspaceSearchStoreActions,
  WorkspaceSearchStoreState,
} from "../../../models/search.ts";

/** One surface's input state. Search results remain derived in the hook. */
export function createWorkspaceSearchStore(
  initial: Pick<WorkspaceSearchStoreState, "filters" | "sort">,
) {
  return createStore<WorkspaceSearchStoreState & WorkspaceSearchStoreActions>()((set) => ({
    query: "",
    filters: initial.filters,
    sort: initial.sort,
    countsRetained: 0,

    setQuery: (query) => set({ query }),
    setSort: (sort) => set({ sort }),
    // Subscribers must never see a cleared query with the previous filters.
    replaceSelection: (filters, query) => set({ query, filters }),

    retainCounts: () => {
      set((state) => ({ countsRetained: state.countsRetained + 1 }));
      let released = false;
      return () => {
        if (released) return;
        released = true;
        set((state) => ({ countsRetained: state.countsRetained - 1 }));
      };
    },
  }));
}
