import { useCallback, useEffect, useLayoutEffect, useState } from "preact/hooks";
import type { RefObject } from "preact";
import { CHAT_FIND_SKIP_SELECTOR } from "../../../config/chat.ts";
import { shortcutService } from "../../../services/platform/shortcutService.ts";
import { chatFindHighlightService } from "../../../services/chat/chatFindHighlightService.ts";
import { domTextSearchService } from "../../../services/platform/domTextSearchService.ts";
import { chatApi } from "../../../api/chatApi";
import type { ConversationSearchHit } from "../../../api/chat/chatTranscriptApi";
import type { ChatEvent } from "../../../models/chat";
import { useDismissShortcut } from "../shared/useDismissShortcut.ts";
import { useShortcut } from "../shared/useShortcut.ts";

/**
 * Search state distinguishes pending and failed requests from empty results.
 */
export type FindStatus =
  | { kind: "idle" }
  | { kind: "empty" }
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "matched"; position: number; total: number };

export interface ChatFind {
  open: boolean;
  query: string;
  status: FindStatus;
  setQuery: (query: string) => void;
  next: () => void;
  previous: () => void;
  close: () => void;
  selectedHit: ConversationSearchHit | null;
  preview: ChatEvent[];
  previewLoading: boolean;
  previewError: string | null;
}

/**
 * Find-in-chat: the query, and where you are in the results.
 *
 * Matches come from the complete active conversation in SQLite. Rendered
 * text also receives highlights, while historical results use a bounded preview.
 * `revision` is any value that changes when the thread's content does, so a
 * match list cannot go stale against a streaming reply.
 */
export function useChatFind({
  chatId,
  scrollRef,
  contentRef,
  revision,
}: {
  chatId: string;
  scrollRef: RefObject<HTMLDivElement>;
  contentRef: RefObject<HTMLDivElement>;
  revision: number;
}): ChatFind {
  const [open, setOpen] = useState(false);
  const [query, setQueryState] = useState("");
  const [index, setIndex] = useState(0);
  const [hits, setHits] = useState<ConversationSearchHit[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);
  const [preview, setPreview] = useState<ChatEvent[]>([]);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const matchCount = hits.length;
  const selectedHit = hits[index] ?? null;

  useEffect(() => {
    let cancelled = false;
    setHits([]);
    setIndex(0);
    setSearchError(null);
    if (!open || !query.trim()) { setSearching(false); return; }
    setSearching(true);
    const timer = setTimeout(() => {
      chatApi.searchTranscript(chatId, query).then((result) => {
        if (!cancelled) setHits(result);
      }).catch((error: Error) => {
        if (!cancelled) setSearchError(error.message);
      }).finally(() => {
        if (!cancelled) setSearching(false);
      });
    }, 250);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [chatId, open, query, revision]);

  // Read one matching turn without loading intervening pages or moving the
  // live thread's cursor. Closing search leaves the current thread intact.
  useEffect(() => {
    let cancelled = false;
    setPreview([]);
    setPreviewError(null);
    if (!open || !selectedHit) { setPreviewLoading(false); return; }
    setPreviewLoading(true);
    chatApi.fetchTranscript(chatId, { before: selectedHit.endSeq + 1, limit: 1 })
      .then((page) => { if (!cancelled) setPreview(page.events); })
      .catch((error: Error) => { if (!cancelled) setPreviewError(error.message); })
      .finally(() => { if (!cancelled) setPreviewLoading(false); });
    return () => { cancelled = true; };
  }, [chatId, open, selectedHit]);

  const close = useCallback(() => setOpen(false), []);

  useShortcut(shortcutService.isFind, (event) => {
    // The browser's own find would otherwise open alongside ours.
    event.preventDefault();
    setOpen(true);
  });

  // Escape closes find from anywhere in the thread, not only from the bar's
  // input, so it undoes Cmd/Ctrl+F wherever the cursor happens to be.
  useDismissShortcut(
    (event) => {
      // The bar's input is usually focused, where Escape can otherwise revert
      // what was typed before the bar closes over it.
      event.preventDefault();
      close();
    },
    { enabled: open }
  );

  // A new query starts from the first match rather than wherever the last one
  // left off.
  useEffect(() => setIndex(0), [query]);

  const setQuery = useCallback((nextQuery: string) => {
    // Clear on the input event itself. Besides avoiding a stale frame before
    // hook effects run, this gives Safari an immediate highlight invalidation
    // when the final character is removed.
    if (nextQuery.trim().length === 0) chatFindHighlightService.clear();
    setQueryState(nextQuery);
  }, []);

  // Keep the browser-owned highlight layers in lockstep with the rendered
  // input. A normal effect runs after paint, which briefly leaves the final
  // one-character match visible when the query changes from "c" to empty.
  useLayoutEffect(() => {
    if (!open || !contentRef.current) {
      chatFindHighlightService.clear();
      return;
    }
    const ranges = domTextSearchService.findRanges(
      contentRef.current,
      query,
      CHAT_FIND_SKIP_SELECTOR
    );
    if (ranges.length === 0) {
      chatFindHighlightService.clear();
      return;
    }
    chatFindHighlightService.show(ranges, ranges[0]);

  }, [open, query, index, revision, contentRef, scrollRef]);

  useEffect(() => () => chatFindHighlightService.clear(), []);

  const step = useCallback(
    (delta: number) =>
      setIndex((current) => (matchCount === 0 ? 0 : (current + delta + matchCount) % matchCount)),
    [matchCount]
  );

  const status: FindStatus =
    searchError ? { kind: "error", message: searchError }
    : searching ? { kind: "loading" }
    : query.trim().length === 0
      ? { kind: "idle" }
      : matchCount === 0
        ? { kind: "empty" }
        : { kind: "matched", position: index + 1, total: matchCount };

  return {
    open,
    query,
    status,
    setQuery,
    next: useCallback(() => step(1), [step]),
    previous: useCallback(() => step(-1), [step]),
    close,
    selectedHit,
    preview,
    previewLoading,
    previewError,
  };
}
