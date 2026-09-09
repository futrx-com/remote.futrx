import type { DateFilter, FacetId, SearchFilters, SortId } from "../../models/search.ts";
import type {
  SearchPreferences, SearchSelectionCommands, SearchSelectionStore,
} from "../../port/workspaceSearch.ts";
import { searchFilterService } from "./searchFilterService.ts";

/** Applies selection rules, publishes atomically, then saves synchronously. */
export class SearchSelectionService implements SearchSelectionCommands {
  private readonly store: SearchSelectionStore;
  private readonly preferences: Pick<SearchPreferences, "writeFilters" | "writeSort">;

  constructor(
    store: SearchSelectionStore,
    preferences: Pick<SearchPreferences, "writeFilters" | "writeSort">,
  ) {
    this.store = store;
    this.preferences = preferences;
  }

  setSort = (sort: SortId): void => {
    this.store.getState().setSort(sort);
    this.preferences.writeSort(sort);
  };

  toggleFacetValue = (facetId: FacetId, value: string): void => {
    this.changeFilters((filters) => searchFilterService.toggleFacetValue(filters, facetId, value));
  };

  setFacetValues = (facetId: FacetId, values: string[]): void => {
    this.changeFilters((filters) => searchFilterService.withFacetValues(filters, facetId, values));
  };

  clearFacet = (facetId: FacetId): void => {
    this.changeFilters((filters) => searchFilterService.clearFacet(filters, facetId));
  };

  setDateFilter = (date: DateFilter): void => {
    this.changeFilters((filters) => searchFilterService.withDate(filters, date));
  };

  clearDate = (): void => {
    this.changeFilters((filters) =>
      searchFilterService.withDate(filters, searchFilterService.clearedDate(filters.date)));
  };

  resetFilters = (): void => {
    this.commitSelection(searchFilterService.defaults());
  };

  clearAll = (): void => {
    this.commitSelection(searchFilterService.defaults(), "");
  };

  private changeFilters(change: (filters: SearchFilters) => SearchFilters): void {
    this.commitSelection(change(this.store.getState().filters));
  }

  private commitSelection(filters: SearchFilters, query = this.store.getState().query): void {
    this.store.getState().replaceSelection(filters, query);
    this.preferences.writeFilters(filters);
  }
}
