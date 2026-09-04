// The vocabulary and shapes of workspace search.
//
// Ids are declared here as ordered tuples with their unions derived, so the
// order is part of the type: it is the order the filter menu lists options in,
// and for `SEARCH_FIELD_IDS` it is also the ranking tie-break rule. The label
// tables in `config/search.ts` and the registries in `workspaceSearchService`
// are keyed by these unions, so the compiler rejects an id that gains a
// vocabulary entry without gaining a definition.

import type { ChatMeta } from "./chat.ts";
import type { ProjectMeta } from "./project.ts";

/** A half-open range of a string, in offsets into the original (unfolded) text. */
export interface MatchSpan {
  start: number;
  end: number;
}

/** One field's keyword match: what it scored, and where it hit. */
export interface FieldMatch {
  score: number;
  spans: MatchSpan[];
}

// ---------------------------------------------------------------------------
// Ordering
// ---------------------------------------------------------------------------

export const SORT_IDS = ["relevance", "recent", "oldest", "title"] as const;

export type SortId = (typeof SORT_IDS)[number];

// ---------------------------------------------------------------------------
// Date filtering
// ---------------------------------------------------------------------------

export const DATE_PRESET_IDS = [
  "any",
  "today",
  "yesterday",
  "7d",
  "30d",
  "90d",
  "custom",
] as const;

export type DatePresetId = (typeof DATE_PRESET_IDS)[number];

export const DATE_FIELD_IDS = ["lastMessageAt", "createdAt"] as const;

export type DateField = (typeof DATE_FIELD_IDS)[number];

export interface DateFilter {
  preset: DatePresetId;
  field: DateField;
  /** ISO `YYYY-MM-DD`, inclusive. Only read when preset is "custom". */
  from?: string;
  /** ISO `YYYY-MM-DD`, inclusive to end-of-day. Only read when preset is "custom". */
  to?: string;
}

/** An inclusive epoch-ms window; `null` on either end means unbounded there. */
export interface ResolvedRange {
  from: number | null;
  to: number | null;
}

// ---------------------------------------------------------------------------
// Facets
// ---------------------------------------------------------------------------

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

export type FacetId = (typeof FACET_IDS)[number];

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

export interface FacetOption {
  value: string;
  label: string;
  /** Secondary text, e.g. a project slug under its name. */
  hint?: string;
}

export type FacetSelections = Record<FacetId, string[]>;

// ---------------------------------------------------------------------------
// The query
// ---------------------------------------------------------------------------

export interface SearchFilters {
  facets: FacetSelections;
  date: DateFilter;
}

// ---------------------------------------------------------------------------
// The index
// ---------------------------------------------------------------------------

// Declaration order is also evaluation order, and ties in the "best field"
// comparison keep the earlier entry — so this order is the tie-break rule.
export const SEARCH_FIELD_IDS = ["title", "project", "path", "skill", "model"] as const;

export type SearchFieldId = (typeof SEARCH_FIELD_IDS)[number];

/**
 * A chat flattened for search: its raw metadata plus the folded text of every
 * searchable field, positionally aligned with `SEARCH_FIELD_IDS`. Built once
 * per chats/projects change so keystrokes never pay normalization cost.
 */
export interface ChatSearchDoc {
  chat: ChatMeta;
  project: ProjectMeta | null;
  folded: string[];
  unread: boolean;
}

// ---------------------------------------------------------------------------
// The results
// ---------------------------------------------------------------------------

/** Which field carried the match, for the "why did this match" line. */
export type MatchedField = SearchFieldId | "none";

export interface SearchHit {
  doc: ChatSearchDoc;
  score: number;
  /** Highlight spans against `chat.title`. Empty when the title didn't match. */
  titleSpans: MatchSpan[];
  matchedField: MatchedField;
}

/** Per-option result counts, computed against every *other* active facet. */
export type FacetCounts = Record<FacetId, Map<string, number>>;

export interface SearchOutcome {
  hits: SearchHit[];
  counts: FacetCounts;
  /**
   * Whether `counts` were actually tallied. Counting is opt-in, and an empty
   * count map otherwise reads the same as one where nothing matched -- callers
   * that narrow by the counts have to tell those apart, and asking the outcome
   * beats carrying the answer alongside it.
   */
  counted: boolean;
  /** Total chats considered before any filtering. */
  total: number;
}

/** One facet resolved for display: its options, what is ticked, and the counts. */
export interface FacetView {
  id: FacetId;
  label: string;
  advanced: boolean;
  emptyHint: string;
  /**
   * What this facet offers, already scoped by the other active filters -- tick
   * Codex and the Model facet offers Codex's models. Narrowing needs the
   * counts, so with every filter menu closed this is the unscoped list; nothing
   * reads it then but the chips, which only look up selected values.
   */
  options: FacetOption[];
  selected: string[];
  counts: Map<string, number>;
}

/**
 * The date window resolved for display. Facets have `FacetView`; without this
 * the date filter was the one part of the selection each surface had to resolve
 * for itself.
 */
export interface DateFilterView {
  /** True when the window narrows the results. */
  active: boolean;
  /** Short label for the active-filter chip. */
  label: string;
}

/**
 * The palette's next step after a key press, resolved by
 * `ui/search/commandPaletteKeyState`. `index` is a row to highlight, not to
 * open: moving the cursor and opening what it sits on are separate presses.
 */
export type CommandPaletteKeyAction =
  | { kind: "ignore" }
  | { kind: "highlight"; index: number }
  | { kind: "open" }
  | { kind: "closeFilters" }
  | { kind: "close" };

// ---------------------------------------------------------------------------
// Boundaries
// ---------------------------------------------------------------------------

/** How one search surface loads and saves its selection. */
export interface SearchPreferences {
  readFilters(): SearchFilters;
  writeFilters(filters: SearchFilters): void;
  readSort(): SortId;
  writeSort(sort: SortId): void;
}

// ---------------------------------------------------------------------------
// The stores
// ---------------------------------------------------------------------------

/**
 * One search surface's selection: the keyword, the filters, the ordering, and
 * how many filter menus are currently asking for per-option counts.
 */
export interface WorkspaceSearchStoreState {
  query: string;
  filters: SearchFilters;
  sort: SortId;
  countsRetained: number;
}

export interface WorkspaceSearchStoreActions {
  setQuery: (query: string) => void;
  setSort: (sort: SortId) => void;
  toggleFacetValue: (facetId: FacetId, value: string) => void;
  setFacetValues: (facetId: FacetId, values: string[]) => void;
  clearFacet: (facetId: FacetId) => void;
  setDateFilter: (date: DateFilter) => void;
  /** Drop the date window, keeping which timestamp the user was asking about. */
  clearDate: () => void;
  resetFilters: () => void;
  /** Drop the keyword and every filter at once. */
  clearAll: () => void;
  /**
   * Ask for per-option facet counts, and release them with the returned
   * function. They are only worth their cost while a filter menu is on screen,
   * and two can be at once -- the sidebar's and the palette's -- so this is a
   * retain count rather than a flag either one could switch off underneath the
   * other. Each release is single-use, so the count cannot fall below the
   * number of menus still open.
   */
  retainCounts: () => () => void;
}

export interface CommandPaletteStoreState {
  open: boolean;
}

export interface CommandPaletteStoreActions {
  openPalette: () => void;
  closePalette: () => void;
  togglePalette: () => void;
}
