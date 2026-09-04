// Where a search surface keeps its filter selection between mounts.
//
// Two implementations of one contract, because the two surfaces want different
// things: the sidebar's search is a place you set up and come back to, so it
// persists across reloads alongside the sidebar's other preferences; the
// palette is a scratch surface you open, use and dismiss, and persisting it
// would both surprise the user and overwrite the sidebar's selection through
// the same key.
//
// Split from searchFilterService for the same reason sidebarPreferenceService
// is split from workspaceSidebarService: this one changes when the storage
// keys or their shape change, not when a filter rule does.
//
// Stored values are treated as untrusted: a hand-edited or stale entry (say, a
// facet that no longer exists) must degrade to "no filter" rather than throw
// during startup. The accepted vocabularies are the ones models/search.ts
// declares, so this file never needs updating when a preset or sort is added.

import { ANY_DATE, DEFAULT_SORT } from "../../config/search.ts";
import { STORAGE_KEYS } from "../../config/storageKeys.ts";
import {
  DATE_FIELD_IDS,
  DATE_PRESET_IDS,
  FACET_IDS,
  SORT_IDS,
} from "../../models/search.ts";
import type {
  DateField,
  DateFilter,
  DatePresetId,
  SearchFilters,
  SearchPreferences,
  SortId,
} from "../../models/search.ts";
import { browserStorageService } from "../platform/browserStorageService.ts";
import { searchFilterService } from "./searchFilterService.ts";

/** Remembered across reloads. What the sidebar's search uses. */
class SearchPreferenceService implements SearchPreferences {
  readFilters(): SearchFilters {
    return this.#parseFilters(browserStorageService.readJson(STORAGE_KEYS.searchFilters));
  }

  writeFilters(filters: SearchFilters): void {
    browserStorageService.writeJson(STORAGE_KEYS.searchFilters, filters);
  }

  readSort(): SortId {
    const stored = browserStorageService.readString(STORAGE_KEYS.searchSort) as SortId | null;
    return stored && SORT_IDS.includes(stored) ? stored : DEFAULT_SORT;
  }

  writeSort(sort: SortId): void {
    browserStorageService.writeString(STORAGE_KEYS.searchSort, sort);
  }

  #parseFilters(raw: unknown): SearchFilters {
    if (!raw || typeof raw !== "object") return searchFilterService.defaults();
    const value = raw as Record<string, unknown>;
    const facets = searchFilterService.emptyFacetSelections();
    const storedFacets = (value.facets ?? {}) as Record<string, unknown>;

    for (const id of FACET_IDS) {
      const selected = storedFacets[id];
      if (!Array.isArray(selected)) continue;
      facets[id] = selected.filter((entry): entry is string => typeof entry === "string");
    }

    return { facets, date: this.#parseDate(value.date) };
  }

  #parseDate(raw: unknown): DateFilter {
    if (!raw || typeof raw !== "object") return { ...ANY_DATE };
    const value = raw as Record<string, unknown>;
    const preset = DATE_PRESET_IDS.includes(value.preset as DatePresetId)
      ? (value.preset as DatePresetId)
      : ANY_DATE.preset;
    const field = DATE_FIELD_IDS.includes(value.field as DateField)
      ? (value.field as DateField)
      : ANY_DATE.field;
    const filter: DateFilter = { preset, field };
    if (typeof value.from === "string") filter.from = value.from;
    if (typeof value.to === "string") filter.to = value.to;
    return filter;
  }
}

/**
 * Starts from the defaults every mount and saves nothing. What the palette
 * uses, so its filters neither outlive the session nor reach into the
 * sidebar's stored selection.
 */
class EphemeralSearchPreferenceService implements SearchPreferences {
  readFilters(): SearchFilters {
    return searchFilterService.defaults();
  }

  writeFilters(_filters: SearchFilters): void {}

  readSort(): SortId {
    return DEFAULT_SORT;
  }

  writeSort(_sort: SortId): void {}
}

export const searchPreferenceService: SearchPreferences = new SearchPreferenceService();

export const ephemeralSearchPreferenceService: SearchPreferences =
  new EphemeralSearchPreferenceService();
