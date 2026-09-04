// What workspace search is tuned to and what it calls things.
//
// The label tables are `Record`s keyed by the id unions in `models/search.ts`,
// so a new id is a compile error here until it is given a name — the guarantee
// the old paired option-table/id-list arrangement was written to provide, now
// enforced by the compiler rather than by both lists being edited together.

import { DAY_MS } from "./time.ts";
import {
  DATE_FIELD_IDS,
  DATE_PRESET_IDS,
  SEARCH_FIELD_IDS,
  SORT_IDS,
} from "../models/search.ts";
import type {
  DateField,
  DateFilter,
  DatePresetId,
  SearchFieldId,
  SortId,
} from "../models/search.ts";

// ---------------------------------------------------------------------------
// Vocabulary the filter menu renders
// ---------------------------------------------------------------------------

const SORT_LABELS: Record<SortId, string> = {
  relevance: "Best match",
  recent: "Newest",
  oldest: "Oldest",
  title: "Title",
};

export const SORT_OPTIONS: readonly { value: SortId; label: string }[] = SORT_IDS.map(
  (value) => ({ value, label: SORT_LABELS[value] })
);

export const DATE_PRESET_LABELS: Record<DatePresetId, string> = {
  any: "Any time",
  today: "Today",
  yesterday: "Yesterday",
  "7d": "Last 7 days",
  "30d": "Last 30 days",
  "90d": "Last 90 days",
  custom: "Custom range",
};

export const DATE_PRESET_OPTIONS: readonly { value: DatePresetId; label: string }[] =
  DATE_PRESET_IDS.map((value) => ({ value, label: DATE_PRESET_LABELS[value] }));

export const DATE_FIELD_LABELS: Record<DateField, string> = {
  lastMessageAt: "Last activity",
  createdAt: "Created",
};

export const DATE_FIELD_OPTIONS: readonly { value: DateField; label: string }[] =
  DATE_FIELD_IDS.map((value) => ({ value, label: DATE_FIELD_LABELS[value] }));

// ---------------------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------------------

export const DEFAULT_SORT: SortId = "relevance";

export const ANY_DATE: DateFilter = { preset: "any", field: "lastMessageAt" };

// ---------------------------------------------------------------------------
// Ranking
// ---------------------------------------------------------------------------

/**
 * Score per matched token, by how directly it matched. The gaps are wide so a
 * stronger tier on one token always outranks a weaker tier on another.
 */
export const MATCH_TIER_SCORES = {
  exact: 120,
  prefix: 90,
  wordStart: 70,
  substring: 45,
  subsequence: 22,
  fuzzy: 14,
} as const;

/** Folded strings kept for reuse. Bounded so a long session cannot grow it without limit. */
export const FOLD_CACHE_LIMIT = 4096;

/** How much a match in each field is worth relative to the title. */
export const SEARCH_FIELD_WEIGHTS: Record<SearchFieldId, number> = {
  title: 1,
  project: 0.55,
  path: 0.4,
  skill: 0.35,
  model: 0.3,
};

/** The one field whose match spans are rendered as highlights on the row. */
export const HIGHLIGHTED_SEARCH_FIELD: SearchFieldId = "title";

/** Index into `ChatSearchDoc.folded` of the field that supplies highlights. */
export const HIGHLIGHTED_FIELD_INDEX = SEARCH_FIELD_IDS.indexOf(HIGHLIGHTED_SEARCH_FIELD);

/** Tie-breakers, kept small so they never outrank a genuinely better match. */
export const RECENCY_WEIGHT = 18;
export const RECENCY_WINDOW_MS = 30 * DAY_MS;

/**
 * What a matching field other than the best one adds. Small, so a chat matching
 * on both its title and its project outranks one matching on either alone
 * without letting weak fields pile up into a better score than a strong one.
 */
export const SECONDARY_FIELD_BONUS = 0.15;

// ---------------------------------------------------------------------------
// What the surfaces show
// ---------------------------------------------------------------------------

/** Enough to fill the palette without rendering hundreds of rows nobody scrolls to. */
export const MAX_PALETTE_RESULTS = 50;

/** Above this many options, a facet's list gets its own filter box. */
export const FACET_INLINE_FILTER_THRESHOLD = 8;
