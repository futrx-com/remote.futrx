import { useEffect, useRef } from "preact/hooks";
import type { ChatSearchHit } from "../../api/chat/chatTranscriptApi";
import type { MessageSearch } from "../../state/hooks/chat/useMessageSearch";
import { MessageHitRow } from "../search/MessageHitRow";

/**
 * Messages matching the sidebar's search, across every chat. Choosing one
 * opens its chat at that message. The next page loads as the list's end
 * scrolls into view.
 */
export function SidebarMessageResults({
  messages,
  chatTitle,
  onOpen,
}: {
  messages: MessageSearch;
  chatTitle: (chatId: string) => string;
  onOpen: (hit: ChatSearchHit) => void;
}) {
  const endRef = useRef<HTMLDivElement>(null);
  const { hasMore, loadingMore, loadMore } = messages;

  useEffect(() => {
    const end = endRef.current;
    if (!end || !hasMore || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) loadMore();
    }, { rootMargin: "240px" });
    observer.observe(end);
    return () => observer.disconnect();
  }, [hasMore, loadingMore, loadMore]);

  if (!messages.loading && messages.hits.length === 0 && !messages.error) return null;

  return (
    <section aria-label="Matching messages" class="pt-2">
      <div class="flex items-baseline justify-between px-2 pb-1 pt-2">
        <span class="text-[10.5px] font-semibold uppercase tracking-[0.1em] text-ink-400">Messages</span>
        <span class="text-[11px] tabular-nums text-ink-400" aria-live="polite">
          {messages.loading
            ? "Searching…"
            : `${messages.hits.length}${messages.hasMore ? "+" : ""}`}
        </span>
      </div>
      {messages.error && <p role="alert" class="px-2 text-[12px] text-accent-red">{messages.error}</p>}
      <div class="space-y-0.5">
        {messages.hits.map((hit) => (
          <MessageHitRow
            key={`${hit.chatId}-${hit.itemId}`}
            hit={hit}
            context={chatTitle(hit.chatId)}
            onSelect={() => onOpen(hit)}
          />
        ))}
      </div>
      {messages.hasMore && (
        <div ref={endRef} class="px-2.5 py-2 text-center text-[11px] text-ink-400">
          {messages.loadingMore ? "Loading more…" : (
            <button type="button" onClick={loadMore} class="hover:text-ink-100">Load more messages</button>
          )}
        </div>
      )}
    </section>
  );
}
