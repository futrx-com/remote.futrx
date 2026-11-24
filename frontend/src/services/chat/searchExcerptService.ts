// Trimming a search excerpt so its first match is in view, even where the
// row clamps the excerpt to a few lines.

import type { MatchSpan } from "../../models/search.ts";

const ELLIPSIS = "… ";

class SearchExcerptService {
  /**
   * Start `text` at most `lead` characters before its first match, at a word
   * boundary, and shift `spans` to match. Text that already starts close
   * enough is returned unchanged.
   */
  focus(text: string, spans: readonly MatchSpan[], lead: number): { text: string; spans: MatchSpan[] } {
    const first = spans.reduce((min, span) => Math.min(min, span.start), Number.POSITIVE_INFINITY);
    if (!Number.isFinite(first) || first <= lead) return { text, spans: [...spans] };
    let start = first - lead;
    // Begin at the next word so the excerpt never opens mid-word.
    const space = text.indexOf(" ", start);
    if (space !== -1 && space < first) start = space + 1;
    const shift = start - ELLIPSIS.length;
    return {
      text: ELLIPSIS + text.slice(start),
      spans: spans.map((span) => ({ start: span.start - shift, end: span.end - shift })),
    };
  }
}

export const searchExcerptService = new SearchExcerptService();
