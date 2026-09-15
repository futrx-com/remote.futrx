// Indexing, keyword scoring, and ordering for workspace search.
// Search fields stay in evaluation order because equal scores keep the first
// match. Facet filtering, counting, and presentation share their own owner.

import { modelShortLabel } from "../../config/chat.ts";
import {
  HIGHLIGHTED_FIELD_INDEX,
  RECENCY_WEIGHT,
  RECENCY_WINDOW_MS,
  SEARCH_FIELD_IDS,
  SEARCH_FIELD_WEIGHTS,
  SECONDARY_FIELD_BONUS,
} from "../../config/search.ts";
import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import type {
  ChatSearchDoc,
  MatchedField,
  MatchSpan,
  SearchFieldId,
  SearchFilters,
  SearchHit,
  SearchOptions,
  SearchOutcome,
  SortId,
} from "../../models/search.ts";
import { textFoldService } from "../platform/textFoldService.ts";
import { textMatchService } from "../platform/textMatchService.ts";
import { searchFilterService } from "./searchFilterService.ts";
import { searchFacetService } from "./searchFacetService.ts";

/** One searchable field: how to read it, and what a match there is worth. */
interface SearchFieldDefinition {
  /** The raw text to search, before folding. */
  textOf(chat: ChatMeta, project: ProjectMeta | null): string;
  /** Why this chat is in the list, when the title alone doesn't show it. */
  describeMatch?(doc: ChatSearchDoc): string;
}

/** The best-matching field's score, its highlight spans, and which field it was. */
interface KeywordScore {
  score: number;
  spans: MatchSpan[];
  field: MatchedField;
}

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

const SEARCH_FIELD_COUNT = SEARCH_FIELD_IDS.length;

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
   * Search each doc once, applying its date window and keyword before facets.
   * This keeps facet counts scoped to the same keyword/date matches as the list.
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
    const facets = searchFacetService.createMatcher(filters.facets, options.withCounts === true);
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
   * Why this chat is in the list, when the title alone doesn't show it. A title
   * match needs no explanation, so it gets none.
   */
  describeMatch(hit: SearchHit): string | null {
    if (hit.matchedField === "none") return null;
    return SEARCH_FIELDS[hit.matchedField].describeMatch?.(hit.doc) ?? null;
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
