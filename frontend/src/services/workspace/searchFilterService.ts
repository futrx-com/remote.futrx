// What the user is asking search for: the facet selection, the date window,
// and every transition between them.
//
// The transitions live here rather than in the hook that holds the selection
// in preact state, so the rules that keep a selection valid are testable and
// have one owner. The hook keeps the lifecycle; this keeps the rules.
//
// Date ranges resolve in the user's local timezone: someone picking "Today"
// means their calendar day, not UTC's. `now` is always injected so the
// resolver stays pure.

import { ANY_DATE, DATE_FIELD_LABELS, DATE_PRESET_LABELS } from "../../config/search.ts";
import { DAY_MS } from "../../config/time.ts";
import { FACET_IDS } from "../../models/search.ts";
import type {
  DateFilter,
  FacetId,
  FacetSelections,
  ResolvedRange,
  SearchFilters,
} from "../../models/search.ts";


class SearchFilterService {
  // -------------------------------------------------------------------------
  // Building a selection
  // -------------------------------------------------------------------------

  emptyFacetSelections(): FacetSelections {
    const selections = {} as FacetSelections;
    for (const id of FACET_IDS) selections[id] = [];
    return selections;
  }

  /** Nothing selected: what a fresh surface starts on and what reset returns to. */
  defaults(): SearchFilters {
    return { facets: this.emptyFacetSelections(), date: { ...ANY_DATE } };
  }

  // -------------------------------------------------------------------------
  // Transitions -- each returns a new selection, never mutating its argument
  // -------------------------------------------------------------------------

  toggleFacetValue(filters: SearchFilters, facetId: FacetId, value: string): SearchFilters {
    const selected = filters.facets[facetId];
    const next = selected.includes(value)
      ? selected.filter((entry) => entry !== value)
      : [...selected, value];
    return this.withFacetValues(filters, facetId, next);
  }

  withFacetValues(filters: SearchFilters, facetId: FacetId, values: string[]): SearchFilters {
    return { ...filters, facets: { ...filters.facets, [facetId]: values } };
  }

  clearFacet(filters: SearchFilters, facetId: FacetId): SearchFilters {
    return this.withFacetValues(filters, facetId, []);
  }

  withDate(filters: SearchFilters, date: DateFilter): SearchFilters {
    return { ...filters, date };
  }

  /** Drop the date window but keep which timestamp the user was asking about. */
  clearedDate(date: DateFilter): DateFilter {
    return { preset: "any", field: date.field };
  }

  // -------------------------------------------------------------------------
  // Reading a selection
  // -------------------------------------------------------------------------

  countActiveFacets(facets: FacetSelections): number {
    let active = 0;
    for (const id of FACET_IDS) if (facets[id].length > 0) active += 1;
    return active;
  }

  /** Every facet that narrows, plus the date window if it does. */
  countActive(filters: SearchFilters): number {
    return this.countActiveFacets(filters.facets) + (this.isDateActive(filters.date) ? 1 : 0);
  }

  isDateActive(filter: DateFilter): boolean {
    if (filter.preset === "any") return false;
    if (filter.preset === "custom") return Boolean(filter.from || filter.to);
    return true;
  }

  /** Short human label for the active-filter chip. */
  describeDate(filter: DateFilter): string {
    const fieldLabel = DATE_FIELD_LABELS[filter.field];
    if (filter.preset === "custom") {
      if (filter.from && filter.to) return `${fieldLabel}: ${filter.from} → ${filter.to}`;
      if (filter.from) return `${fieldLabel}: after ${filter.from}`;
      if (filter.to) return `${fieldLabel}: before ${filter.to}`;
      return `${fieldLabel}: custom`;
    }
    return `${fieldLabel}: ${DATE_PRESET_LABELS[filter.preset]}`;
  }

  // -------------------------------------------------------------------------
  // Resolving a date window
  // -------------------------------------------------------------------------

  /** Turn a filter into an inclusive epoch-ms window, unbounded where null. */
  resolveDateRange(filter: DateFilter, now: number): ResolvedRange {
    const today = this.#startOfLocalDay(now);

    switch (filter.preset) {
      case "today":
        return { from: today, to: today + DAY_MS - 1 };
      case "yesterday":
        return { from: today - DAY_MS, to: today - 1 };
      case "7d":
        return { from: today - 6 * DAY_MS, to: null };
      case "30d":
        return { from: today - 29 * DAY_MS, to: null };
      case "90d":
        return { from: today - 89 * DAY_MS, to: null };
      case "custom": {
        const from = this.#parseLocalDate(filter.from);
        const parsedTo = this.#parseLocalDate(filter.to);
        // An end date is inclusive of the whole day the user picked.
        const to = parsedTo === null ? null : parsedTo + DAY_MS - 1;
        // A backwards range would silently match nothing; swap instead.
        if (from !== null && to !== null && from > to) {
          return { from: to - DAY_MS + 1, to: from + DAY_MS - 1 };
        }
        return { from, to };
      }
      case "any":
      default:
        return { from: null, to: null };
    }
  }

  inRange(at: number, range: ResolvedRange): boolean {
    if (range.from !== null && at < range.from) return false;
    if (range.to !== null && at > range.to) return false;
    return true;
  }

  #startOfLocalDay(at: number): number {
    const date = new Date(at);
    date.setHours(0, 0, 0, 0);
    return date.getTime();
  }

  /** Parse `YYYY-MM-DD` as local midnight. Returns null for anything malformed. */
  #parseLocalDate(value: string | undefined): number | null {
    if (!value) return null;
    const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value.trim());
    if (!match) return null;
    const year = Number(match[1]);
    const month = Number(match[2]);
    const day = Number(match[3]);
    if (month < 1 || month > 12 || day < 1 || day > 31) return null;
    const date = new Date(year, month - 1, day, 0, 0, 0, 0);
    // Reject roll-over dates like 2026-02-31.
    if (
      date.getFullYear() !== year ||
      date.getMonth() !== month - 1 ||
      date.getDate() !== day
    ) {
      return null;
    }
    return date.getTime();
  }
}

export const searchFilterService = new SearchFilterService();
