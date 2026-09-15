// Workspace search vocabulary, defaults, and ranking settings.
// Model unions derive from these ordered catalogs, so label tables and service
// registries remain exhaustive without duplicating the accepted ids.

import { DAY_MS } from "./time.ts";
import type {
  DateField,
  DateFilter,
  DatePresetId,
  SearchFieldId,
  SortId,
} from "../models/search.ts";

// ---------------------------------------------------------------------------
// Ordered catalogs and sentinel values
// ---------------------------------------------------------------------------

export const SORT_IDS = ["relevance", "recent", "oldest", "title"] as const;

export const DATE_PRESET_IDS = [
  "any",
  "today",
  "yesterday",
  "7d",
  "30d",
  "90d",
  "custom",
] as const;

export const DATE_FIELD_IDS = ["lastMessageAt", "createdAt"] as const;

export const FACET_IDS = [
  "project",
  "provider",
  "model",
  "mode",
  "status",
  "effort",
  "tier",
  "skill",
] as const;

/** Sentinel for chats that belong to no project, so it can be a normal option. */
export const UNASSIGNED_PROJECT = " unassigned";

export const STATUS_UNREAD = "unread";

export const STATUS_RUNNING = "running";

/**
 * The facet value standing for "this chat recorded nothing here" — no provider,
 * no model, no mode. A sentinel rather than an absence, so an unset field is a
 * tickable option like any other instead of a hole in the list.
 */
export const UNSET_FACET_VALUE = "";

// Declaration order is also evaluation order, and ties in the "best field"
// comparison keep the earlier entry — so this order is the tie-break rule.
export const SEARCH_FIELD_IDS = ["title", "project", "path", "skill", "model"] as const;

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
