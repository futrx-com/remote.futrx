// Ranked search over the workspace's chats: what is indexed, what filters it,
// how it is ordered, and the per-option counts the filter menu shows.
//
// Two registries drive everything. `#fields` declares what a keyword is matched
// against and what a match there is worth; `#facets` declares what can filter,
// how a doc's values are read, and how they are labelled. Adding either is one
// entry, not an edit to the scoring loop, the filter menu, the chips, the badge
// count and the reset behaviour.
//
// Both are keyed by the id unions in models/search.ts, so a declared id with no
// definition is a compile error, and both are ordered by the id tuples there --
// which for fields is also the ranking tie-break rule.

import { modelShortLabel, providerDisplayLabel } from "../../config/chat.ts";
import { capitalize } from "../../config/text.ts";
import {
  HIGHLIGHTED_FIELD_INDEX,
  RECENCY_WEIGHT,
  RECENCY_WINDOW_MS,
  SEARCH_FIELD_WEIGHTS,
  SECONDARY_FIELD_BONUS,
} from "../../config/search.ts";
import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import {
  FACET_IDS,
  SEARCH_FIELD_IDS,
  STATUS_RUNNING,
  STATUS_UNREAD,
  UNASSIGNED_PROJECT,
  UNSET_FACET_VALUE,
} from "../../models/search.ts";
import type {
  ChatSearchDoc,
  FacetCounts,
  FacetId,
  FacetOption,
  FacetSelections,
  FacetView,
  MatchedField,
  MatchSpan,
  SearchFieldId,
  SearchFilters,
  SearchHit,
  SearchOutcome,
  SortId,
} from "../../models/search.ts";
import { textFoldService } from "../platform/textFoldService.ts";
import { textMatchService } from "../platform/textMatchService.ts";
import { searchFilterService } from "./searchFilterService.ts";

/** One searchable field: how to read it, and what a match there is worth. */
interface SearchFieldDefinition {
  /** The raw text to search, before folding. */
  textOf(chat: ChatMeta, project: ProjectMeta | null): string;
  /** Why this chat is in the list, when the title alone doesn't show it. */
  describeMatch?(doc: ChatSearchDoc): string;
}

/** One filter: how to read a doc's values for it, and how to name them. */
interface FacetDefinition {
  readonly label: string;
  /** Advanced facets start collapsed behind a disclosure. */
  readonly advanced: boolean;
  /** Empty-state copy when no chat carries a value for this facet. */
  readonly emptyHint: string;
  /**
   * The doc's values for this facet. A doc passes when the selection is empty
   * or intersects this list. Also drives per-option counting.
   */
  valuesOf(doc: ChatSearchDoc): readonly string[];
  /** Human label for a raw value, used in both the menu and the chips. */
  labelFor(value: string, doc?: ChatSearchDoc): string;
  /** Optional secondary line in the menu (e.g. a project slug). */
  hintFor?(value: string, doc?: ChatSearchDoc): string | undefined;
}

/** The best-matching field's score, its highlight spans, and which field it was. */
interface KeywordScore {
  score: number;
  spans: MatchSpan[];
  field: MatchedField;
}


// Field and facet options are derived from the chats that actually exist rather
// than from a catalog, so the menu never offers a provider or model the user
// has never used, and free-form model strings show up without a code change.
const SEARCH_FIELDS: Record<SearchFieldId, SearchFieldDefinition> = {
  title: {
    textOf: (chat) => chat.title || "",
  },
  project: {
    textOf: (_chat, project) => (project ? `${project.name} ${project.slug}` : ""),
    describeMatch: (doc) => (doc.project ? `project · ${doc.project.name}` : "project"),
  },
  path: {
    textOf: (chat) => chat.cwd || "",
    describeMatch: (doc) => (doc.chat.cwd ? `path · ${doc.chat.cwd}` : "path"),
  },
  skill: {
    textOf: (chat) => chat.selectedSkills?.map((skill) => skill.name).join(" ") ?? "",
    describeMatch: () => "skill",
  },
  model: {
    textOf: (chat) => chat.model || "",
    describeMatch: (doc) => `model · ${modelShortLabel(doc.chat.model)}`,
  },
};

const FACETS: Record<FacetId, FacetDefinition> = {
  project: {
    label: "Projects",
    advanced: false,
    emptyHint: "No projects yet",
    // Keyed off the *resolved* project, not the raw id: a chat left pointing at
    // a deleted project would otherwise invent a filter option labelled with a
    // bare id, for a project nobody created. It groups with the unassigned.
    valuesOf: (doc) => [doc.project?.id ?? UNASSIGNED_PROJECT],
    labelFor: (value, doc) =>
      value === UNASSIGNED_PROJECT ? "Unassigned chats" : doc?.project?.name || value,
    hintFor: (value, doc) =>
      value === UNASSIGNED_PROJECT ? undefined : doc?.project?.slug,
  },
  provider: {
    label: "Provider",
    advanced: false,
    emptyHint: "No providers recorded",
    valuesOf: (doc) => [doc.chat.provider || UNSET_FACET_VALUE],
    labelFor: (value) => (value === UNSET_FACET_VALUE ? "Default" : providerDisplayLabel(value)),
  },
  model: {
    label: "Model",
    advanced: false,
    emptyHint: "No models recorded",
    valuesOf: (doc) => [doc.chat.model || UNSET_FACET_VALUE],
    labelFor: (value) => (value === UNSET_FACET_VALUE ? "Auto" : modelShortLabel(value)),
  },
  mode: {
    label: "Mode",
    advanced: false,
    emptyHint: "No modes recorded",
    valuesOf: (doc) => [doc.chat.mode || UNSET_FACET_VALUE],
    labelFor: (value) => (value === UNSET_FACET_VALUE ? "Unset" : capitalize(value)),
  },
  status: {
    label: "Status",
    advanced: false,
    emptyHint: "No status to filter",
    valuesOf: (doc) => {
      const values: string[] = [];
      if (doc.unread) values.push(STATUS_UNREAD);
      if (doc.chat.running) values.push(STATUS_RUNNING);
      return values;
    },
    labelFor: (value) => (value === STATUS_RUNNING ? "Running" : "Unread"),
  },
  effort: {
    label: "Reasoning effort",
    advanced: true,
    emptyHint: "No effort levels recorded",
    valuesOf: (doc) => [doc.chat.reasoningEffort || UNSET_FACET_VALUE],
    labelFor: (value) => (value === UNSET_FACET_VALUE ? "Auto" : capitalize(value)),
  },
  tier: {
    label: "Service tier",
    advanced: true,
    emptyHint: "No service tiers recorded",
    valuesOf: (doc) => [doc.chat.serviceTier || UNSET_FACET_VALUE],
    labelFor: (value) => (value === UNSET_FACET_VALUE ? "Auto" : capitalize(value)),
  },
  skill: {
    label: "Skills",
    advanced: true,
    emptyHint: "No skills selected in any chat",
    valuesOf: (doc) => doc.chat.selectedSkills?.map((skill) => skill.name) ?? [],
    labelFor: (value) => value,
  },
};

const FACET_COUNT = FACET_IDS.length;
const ALL_FACETS_MASK = (1 << FACET_COUNT) - 1;
const SEARCH_FIELD_COUNT = SEARCH_FIELD_IDS.length;

/**
 * Applies the facet selection to each doc and, optionally, tallies how many
 * docs each option would match.
 *
 * The selection sets, the per-facet flags, and the scratch space for a doc's
 * values are one unit of state that must stay index-aligned with `FACET_IDS`,
 * so they live together rather than as parallel arrays threaded through the
 * search loop. Nothing here allocates per doc: the scratch array is reused, and
 * a facet's values are only materialized when it actually filters or is being
 * counted.
 */
class FacetMatcher {
  readonly #selections: ReadonlySet<string>[] = new Array(FACET_COUNT);
  readonly #active: boolean[] = new Array(FACET_COUNT);
  readonly #needValues: boolean[] = new Array(FACET_COUNT);
  readonly #docValues: (readonly string[] | null)[] = new Array(FACET_COUNT).fill(null);
  readonly #counts: FacetCounts;
  readonly #withCounts: boolean;

  constructor(facets: FacetSelections, withCounts: boolean) {
    this.#withCounts = withCounts;
    this.#counts = {} as FacetCounts;

    for (let i = 0; i < FACET_COUNT; i += 1) {
      const selected = facets[FACET_IDS[i]];
      this.#selections[i] = new Set(selected);
      this.#active[i] = selected.length > 0;
      this.#needValues[i] = this.#active[i] || withCounts;
      this.#counts[FACET_IDS[i]] = new Map<string, number>();
    }
  }

  /** Every option's match count, valid once every doc has been `accepts`-ed. */
  get counts(): FacetCounts {
    return this.#counts;
  }

  /** True when the doc satisfies every facet. Tallies counts as a side effect. */
  accepts(doc: ChatSearchDoc): boolean {
    let mask = 0;
    for (let i = 0; i < FACET_COUNT; i += 1) {
      if (!this.#needValues[i]) {
        // Nothing selected and no counting: this facet cannot exclude anything.
        mask |= 1 << i;
        this.#docValues[i] = null;
        continue;
      }
      const values = FACETS[FACET_IDS[i]].valuesOf(doc);
      this.#docValues[i] = values;
      if (!this.#active[i] || this.#intersects(values, this.#selections[i])) mask |= 1 << i;
    }

    if (this.#withCounts) this.#tally(mask);
    return mask === ALL_FACETS_MASK;
  }

  /**
   * A doc counts toward facet i's options when it passes every *other* facet,
   * so the numbers show what each checkbox would actually add.
   */
  #tally(mask: number): void {
    for (let i = 0; i < FACET_COUNT; i += 1) {
      const others = ALL_FACETS_MASK & ~(1 << i);
      if ((mask & others) !== others) continue;
      const values = this.#docValues[i];
      if (!values) continue;
      const bucket = this.#counts[FACET_IDS[i]];
      for (let v = 0; v < values.length; v += 1) {
        bucket.set(values[v], (bucket.get(values[v]) ?? 0) + 1);
      }
    }
  }

  #intersects(values: readonly string[], selection: ReadonlySet<string>): boolean {
    for (let i = 0; i < values.length; i += 1) if (selection.has(values[i])) return true;
    return false;
  }
}

/** Compute per-option facet counts. Only an open filter menu needs these. */
export interface SearchOptions {
  withCounts?: boolean;
}

class WorkspaceSearchService {
  /**
   * Flatten chats into search docs, pre-folding every searchable field.
   *
   * This runs once per chats/projects change -- never per keystroke -- so
   * typing only ever pays for comparison, not normalization.
   */
  buildIndex(
    chats: readonly ChatMeta[],
    projects: readonly ProjectMeta[]
  ): ChatSearchDoc[] {
    const projectsById = new Map(projects.map((project) => [project.id, project]));

    return chats.map((chat) => {
      const project = (chat.projectId && projectsById.get(chat.projectId)) || null;
      return {
        chat,
        project,
        folded: SEARCH_FIELD_IDS.map((id) =>
          textFoldService.fold(SEARCH_FIELDS[id].textOf(chat, project))
        ),
        unread: (chat.lastMessageAt || 0) > (chat.lastReadAt || 0),
      };
    });
  }

  /**
   * Everything happens in a single pass over the docs. Facet predicates are
   * cheap (Set lookups and integer compares) and run before keyword scoring, so
   * narrowing to one project means the expensive stage only sees that project's
   * chats.
   */
  run(
    docs: readonly ChatSearchDoc[],
    filters: SearchFilters,
    query: string,
    sort: SortId,
    now: number,
    options: SearchOptions = {}
  ): SearchOutcome {
    const tokens = textMatchService.tokenize(query);
    const scoring = tokens.length > 0;
    const facets = new FacetMatcher(filters.facets, options.withCounts === true);
    const hits: SearchHit[] = [];

    const range = searchFilterService.isDateActive(filters.date)
      ? searchFilterService.resolveDateRange(filters.date, now)
      : null;
    const dateField = filters.date.field;

    for (let d = 0; d < docs.length; d += 1) {
      const doc = docs[d];

      if (range) {
        const at = dateField === "createdAt" ? doc.chat.createdAt : doc.chat.lastMessageAt;
        if (!searchFilterService.inRange(at || 0, range)) continue;
      }

      let keyword: KeywordScore | null = null;
      if (scoring) {
        keyword = this.#scoreDoc(doc, tokens);
        if (!keyword) continue;
      }

      // Counting happens inside `accepts`, so it sees only docs that already
      // passed the date window and the keyword -- the same set the list shows.
      if (!facets.accepts(doc)) continue;

      hits.push({
        doc,
        score: (keyword?.score ?? 0) + this.#recencyBoost(doc.chat.lastMessageAt || 0, now),
        titleSpans: keyword?.spans ?? [],
        matchedField: keyword?.field ?? "none",
      });
    }

    const effectiveSort: SortId = sort === "relevance" && !scoring ? "recent" : sort;
    hits.sort((left, right) => this.#compareHits(left, right, effectiveSort));

    return {
      hits,
      counts: facets.counts,
      counted: options.withCounts === true,
      total: docs.length,
    };
  }

  /**
   * Resolve every facet for display against one search outcome.
   *
   * One facet's view is three things joined: the values in use (with their
   * labels), how many chats each would match given the *other* filters, and
   * which are ticked. Options are scoped by the other active filters, so
   * ticking Codex leaves the Model facet offering Codex's models. That scoping
   * is derived from the counts, which are only tallied while a filter menu is
   * open; with none open the unscoped list stands in, since the only thing
   * reading it then is the chips, and they only look up values that are already
   * selected.
   */
  facetViews(
    docs: readonly ChatSearchDoc[],
    filters: SearchFilters,
    outcome: SearchOutcome
  ): FacetView[] {
    return FACET_IDS.map((id) => {
      const facet = FACETS[id];
      const selected = filters.facets[id];
      const counts = outcome.counts[id];
      const inUse = this.#optionsForFacet(id, docs);
      return {
        id,
        label: facet.label,
        advanced: facet.advanced,
        emptyHint: facet.emptyHint,
        options: outcome.counted ? this.#offerableOptions(inUse, counts, selected) : inUse,
        selected,
        counts,
      };
    });
  }

  /**
   * Why this chat is in the list, when the title alone doesn't show it. A title
   * match needs no explanation, so it gets none.
   */
  describeMatch(hit: SearchHit): string | null {
    if (hit.matchedField === "none") return null;
    return SEARCH_FIELDS[hit.matchedField].describeMatch?.(hit.doc) ?? null;
  }

  /**
   * Build the selectable options for one facet from the docs themselves,
   * keeping one representative doc per value so labels can consult sibling
   * metadata (a project's name, a chat's provider when labelling its model).
   */
  #optionsForFacet(id: FacetId, docs: readonly ChatSearchDoc[]): FacetOption[] {
    const facet = FACETS[id];
    const representatives = new Map<string, ChatSearchDoc>();
    for (const doc of docs) {
      for (const value of facet.valuesOf(doc)) {
        if (!representatives.has(value)) representatives.set(value, doc);
      }
    }

    const options: FacetOption[] = [];
    for (const [value, doc] of representatives) {
      options.push({
        value,
        label: facet.labelFor(value, doc),
        hint: facet.hintFor?.(value, doc),
      });
    }

    // Unassigned sorts last; everything else alphabetically by label.
    options.sort((left, right) => {
      if (left.value === UNASSIGNED_PROJECT) return 1;
      if (right.value === UNASSIGNED_PROJECT) return -1;
      return left.label.localeCompare(right.label);
    });
    return options;
  }

  /**
   * The options worth offering: those some chat would still match given every
   * *other* active filter, plus whatever is already selected so a selection can
   * always be seen and cleared.
   *
   * Scoping one facet by the others is what makes the per-provider facets
   * behave. Models and modes belong to a provider, so with Codex ticked the
   * Model list should be Codex's models -- an option that would match nothing
   * is not a choice, it is noise, and offering every model ever used invites a
   * pair of filters that can never agree.
   *
   * `counts` already carries exactly that set: the run tallies a value only
   * from docs that pass every facet but this one.
   */
  #offerableOptions(
    options: readonly FacetOption[],
    counts: ReadonlyMap<string, number>,
    selected: readonly string[]
  ): FacetOption[] {
    const keepAnyway = new Set(selected);
    return options.filter(
      (option) => keepAnyway.has(option.value) || (counts.get(option.value) ?? 0) > 0
    );
  }

  /**
   * Score one doc against the query across every searchable field, or null when
   * none of them match.
   *
   * The best-matching field counts at full weight and every other matching
   * field adds a small bonus, so a chat matching on both its title and its
   * project outranks one matching on either alone without letting weak fields
   * pile up into a better score than a strong one.
   */
  #scoreDoc(doc: ChatSearchDoc, tokens: string[]): KeywordScore | null {
    let best = -1;
    let total = 0;
    let spans: MatchSpan[] = [];
    let field: MatchedField = "none";

    for (let f = 0; f < SEARCH_FIELD_COUNT; f += 1) {
      const hit = textMatchService.matchField(doc.folded[f], tokens);
      if (!hit) continue;

      const value = hit.score * SEARCH_FIELD_WEIGHTS[SEARCH_FIELD_IDS[f]];
      total += value;
      // Strictly greater, so ties keep the earlier -- higher-weighted -- field.
      if (value > best) {
        best = value;
        field = SEARCH_FIELD_IDS[f];
      }
      if (f === HIGHLIGHTED_FIELD_INDEX) spans = hit.spans;
    }

    if (best < 0) return null;
    return { score: best + (total - best) * SECONDARY_FIELD_BONUS, spans, field };
  }

  #recencyBoost(lastMessageAt: number, now: number): number {
    const age = now - lastMessageAt;
    if (age <= 0) return RECENCY_WEIGHT;
    if (age >= RECENCY_WINDOW_MS) return 0;
    return RECENCY_WEIGHT * (1 - age / RECENCY_WINDOW_MS);
  }

  #compareHits(left: SearchHit, right: SearchHit, sort: SortId): number {
    switch (sort) {
      case "recent":
        return right.doc.chat.lastMessageAt - left.doc.chat.lastMessageAt;
      case "oldest":
        return left.doc.chat.lastMessageAt - right.doc.chat.lastMessageAt;
      case "title":
        return (left.doc.chat.title || "").localeCompare(right.doc.chat.title || "");
      case "relevance":
      default:
        if (right.score !== left.score) return right.score - left.score;
        return right.doc.chat.lastMessageAt - left.doc.chat.lastMessageAt;
    }
  }
}

export const workspaceSearchService = new WorkspaceSearchService();
