import type { MatchSpan } from "../../models/search";

/**
 * Render text with the matched ranges emphasized. Spans are offsets into the
 * original string — the search index folds text without changing its length so
 * these stay aligned.
 */
export function HighlightedText({
  text,
  spans,
  class: className,
}: {
  text: string;
  spans: readonly MatchSpan[];
  class?: string;
}) {
  if (spans.length === 0) return <span class={className}>{text}</span>;

  const parts: preact.ComponentChildren[] = [];
  let cursor = 0;
  for (let i = 0; i < spans.length; i += 1) {
    const span = spans[i];
    const start = Math.max(0, Math.min(span.start, text.length));
    const end = Math.max(start, Math.min(span.end, text.length));
    if (start > cursor) parts.push(text.slice(cursor, start));
    if (end > start) {
      parts.push(
        <mark
          key={`${start}-${end}`}
          class="bg-accent-blue/25 text-accent-blue rounded-[2px] px-px"
        >
          {text.slice(start, end)}
        </mark>
      );
    }
    cursor = end;
  }
  if (cursor < text.length) parts.push(text.slice(cursor));

  return <span class={className}>{parts}</span>;
}
