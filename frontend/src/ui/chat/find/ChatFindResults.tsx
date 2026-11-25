import { useEffect, useRef } from "preact/hooks";
import type { ConversationSearchHit } from "../../../api/chat/chatTranscriptApi";
import { MessageHitRow } from "../../search/MessageHitRow";

/** Distance from the list's end, in pixels, at which the next page loads. */
const LOAD_MORE_MARGIN = 240;

/**
 * Every find-in-chat match at once, each with who sent it, when, and a
 * clamped excerpt. Choosing one jumps the thread to its message.
 */
export function ChatFindResults({
  hits,
  selectedIndex,
  hasMore,
  loadingMore,
  onLoadMore,
  onSelect,
}: {
  hits: readonly ConversationSearchHit[];
  selectedIndex: number;
  hasMore: boolean;
  loadingMore: boolean;
  /** Called as the list nears its end while older matches remain. */
  onLoadMore: () => void;
  onSelect: (index: number) => void;
}) {
  const listRef = useRef<HTMLUListElement>(null);

  // Keep the row chosen with Enter or the arrows visible in the list.
  useEffect(() => {
    listRef.current
      ?.querySelector<HTMLElement>(`[data-find-result="${selectedIndex}"]`)
      ?.scrollIntoView({ block: "nearest" });
  }, [selectedIndex]);

  const maybeLoadMore = () => {
    const list = listRef.current;
    if (!list || !hasMore || loadingMore) return;
    if (list.scrollHeight - list.scrollTop - list.clientHeight < LOAD_MORE_MARGIN) onLoadMore();
  };

  // A first page shorter than the list leaves nothing to scroll; fill it.
  useEffect(maybeLoadMore, [hits.length, hasMore, loadingMore]);

  return (
    <ul
      ref={listRef}
      onScroll={maybeLoadMore}
      aria-label="Search results"
      class="mt-1.5 flex max-h-[min(60vh,32rem)] w-[min(34rem,calc(100vw-3rem))] flex-col gap-1
             overflow-y-auto overscroll-contain border-t border-line pt-1.5 scrollbar-thin"
    >
      {hits.map((hit, index) => (
        <li key={hit.itemId}>
          <MessageHitRow
            hit={hit}
            selected={index === selectedIndex}
            onSelect={() => onSelect(index)}
            data-find-result={index}
          />
        </li>
      ))}
      {hasMore && (
        <li class="px-2.5 py-2 text-center text-[11px] text-ink-400" aria-live="polite">
          {loadingMore ? "Loading more…" : "Scroll for older matches"}
        </li>
      )}
    </ul>
  );
}
