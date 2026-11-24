import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import type { RefObject } from "preact";
import { CHAT_SEARCH_PAGE_LIMIT } from "../../../config/api.ts";
import { shortcutService } from "../../../services/platform/shortcutService.ts";
import { chatFindHighlightService } from "../../../services/chat/chatFindHighlightService.ts";
import { chatApi } from "../../../api/chatApi";
import type { ConversationSearchHit } from "../../../api/chat/chatTranscriptApi";
import { useDismissShortcut } from "../shared/useDismissShortcut.ts";
import { useShortcut } from "../shared/useShortcut.ts";
import { useChatFindHighlights } from "./useChatFindHighlights";

/**
 * Search state distinguishes pending and failed requests from empty results.
 */
export type FindStatus =
  | { kind: "idle" }
  | { kind: "empty" }
  | { kind: "loading" }
  | { kind: "error"; message: string }
  /** `position` is 0 until a match has been jumped to; `more` means older
   *  matches exist beyond those loaded. */
  | { kind: "matched"; position: number; total: number; more: boolean };

export interface ChatFind {
  open: boolean;
  query: string;
  status: FindStatus;
  setQuery: (query: string) => void;
  next: () => void;
  previous: () => void;
  close: () => void;
  /** Matches loaded so far, newest first. */
  hits: ConversationSearchHit[];
  /** Older matches exist beyond `hits`. */
  hasMore: boolean;
  loadingMore: boolean;
  /** Load the next page of older matches. */
  loadMore: () => void;
  /** The match last jumped to, or -1 before the first jump. */
  selectedIndex: number;
  /** Jump the thread to match `index`, loading older history if needed. */
  select: (index: number) => void;
  /** Open find on `query` and jump to `hit`, as a search elsewhere asks. */
  openAt: (query: string, hit: ConversationSearchHit) => void;
  /** The result list is shown; it folds away after a jump so it does not
   *  cover the message, and a new query brings it back. */
  listOpen: boolean;
  setListOpen: (open: boolean) => void;
}

/**
 * Find-in-chat: the query, and where you are in the results.
 *
 * Matches come from the complete active conversation in SQLite and are
 * listed with their excerpts. Choosing one brings its message into the
 * thread, loading older history through `revealHit` when it is not rendered
 * yet. `revision` is any value that changes when the thread's content does,
 * so a match list cannot go stale against a streaming reply.
 */
export function useChatFind({
  chatId,
  scrollRef,
  contentRef,
  revision,
  revealHit,
}: {
  chatId: string;
  scrollRef: RefObject<HTMLDivElement>;
  contentRef: RefObject<HTMLDivElement>;
  revision: number;
  /** Make the message holding `hit` part of the rendered thread. */
  revealHit: (hit: ConversationSearchHit) => Promise<void>;
}): ChatFind {
  const [open, setOpen] = useState(false);
  const [query, setQueryState] = useState("");
  const [index, setIndex] = useState(-1);
  const [hits, setHits] = useState<ConversationSearchHit[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  // A step past the last loaded match waits for the next page, then lands here.
  const pendingSelectRef = useRef<number | null>(null);
  const matchCount = hits.length;
  const [listOpen, setListOpen] = useState(true);
  // Bumped by `openAt` so a repeated query still searches afresh.
  const [searchNonce, setSearchNonce] = useState(0);
  // A hit `openAt` asked for, jumped to once its query's results arrive.
  const pendingHitRef = useRef<ConversationSearchHit | null>(null);
  const jumpToRef = useRef<(hit: ConversationSearchHit) => Promise<void>>(async () => {});

  const hitsRef = useRef(hits);
  hitsRef.current = hits;
  // Only a new query starts over. A thread change (a streamed reply, or older
  // history loaded by a jump) refreshes the matches in place and keeps the
  // selected one.
  const searchKey = `${chatId}\u0000${open}\u0000${query}\u0000${searchNonce}`;
  const searchKeyRef = useRef("");

  useEffect(() => {
    let cancelled = false;
    const fresh = searchKeyRef.current !== searchKey;
    searchKeyRef.current = searchKey;
    if (fresh) {
      setHits([]);
      setIndex(-1);
      setSearchError(null);
      setHasMore(false);
      pendingSelectRef.current = null;
    }
    if (!open || !query.trim()) { setSearching(false); return; }
    if (fresh) setSearching(true);
    const timer = setTimeout(() => {
      chatApi.searchTranscript(chatId, query).then((result) => {
        if (cancelled) return;
        if (fresh) {
          setHits(result);
          setHasMore(result.length >= CHAT_SEARCH_PAGE_LIMIT);
          const pending = pendingHitRef.current;
          if (pending) {
            pendingHitRef.current = null;
            setIndex(result.findIndex((hit) => hit.itemId === pending.itemId));
            setListOpen(false);
            void jumpToRef.current(pending);
          }
          return;
        }
        // A refresh re-reads the newest page; merge it over the pages
        // already loaded so the list and its selection stay put.
        const previous = hitsRef.current;
        const merged = mergeHits(result, previous);
        setIndex((current) => {
          const selected = previous[current]?.itemId;
          return selected === undefined ? -1 : merged.findIndex((hit) => hit.itemId === selected);
        });
        setHits(merged);
      }).catch((error: Error) => {
        if (!cancelled) setSearchError(error.message);
      }).finally(() => {
        if (!cancelled) setSearching(false);
      });
    }, 250);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [searchKey, chatId, open, query, revision]);

  const loadMore = useCallback(() => {
    const oldest = hitsRef.current[hitsRef.current.length - 1];
    if (!oldest || !hasMore || loadingMore) return;
    const key = searchKey;
    setLoadingMore(true);
    chatApi.searchTranscript(chatId, query, oldest.startSeq).then((page) => {
      if (searchKeyRef.current !== key) return;
      setHits((current) => mergeHits(current, page));
      setHasMore(page.length >= CHAT_SEARCH_PAGE_LIMIT);
    }).catch((error: Error) => {
      if (searchKeyRef.current === key) setSearchError(error.message);
    }).finally(() => setLoadingMore(false));
  }, [chatId, hasMore, loadingMore, query, searchKey]);

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
  useEffect(() => {
    setIndex(-1);
    setListOpen(true);
  }, [query, open]);

  const setQuery = useCallback((nextQuery: string) => {
    // Clear on the input event itself. Besides avoiding a stale frame before
    // hook effects run, this gives Safari an immediate highlight invalidation
    // when the final character is removed.
    if (nextQuery.trim().length === 0) chatFindHighlightService.clear();
    setQueryState(nextQuery);
  }, []);

  const jumpTo = useChatFindHighlights({
    contentRef,
    scrollRef,
    open,
    query,
    selectedIndex: index,
    revision,
    revealHit,
  });
  jumpToRef.current = jumpTo;

  const select = useCallback((next: number) => {
    const hit = hits[next];
    if (!hit) return;
    setIndex(next);
    setListOpen(false);
    void jumpTo(hit);
  }, [hits, jumpTo]);

  const openAt = useCallback((nextQuery: string, hit: ConversationSearchHit) => {
    pendingHitRef.current = hit;
    setSearchNonce((nonce) => nonce + 1);
    setOpen(true);
    setQueryState(nextQuery);
  }, []);

  useEffect(() => {
    const pending = pendingSelectRef.current;
    if (pending === null || !hits[pending]) return;
    pendingSelectRef.current = null;
    select(pending);
  }, [hits, select]);

  const step = useCallback(
    (delta: number) => {
      if (matchCount === 0) return;
      if (delta > 0 && index === matchCount - 1 && hasMore) {
        pendingSelectRef.current = index + 1;
        loadMore();
        return;
      }
      // Before any jump, Enter goes to the newest match and Shift+Enter to the oldest.
      select(index < 0 ? (delta > 0 ? 0 : matchCount - 1) : (index + delta + matchCount) % matchCount);
    },
    [hasMore, index, loadMore, matchCount, select]
  );

  const status: FindStatus =
    searchError ? { kind: "error", message: searchError }
    : searching ? { kind: "loading" }
    : query.trim().length === 0
      ? { kind: "idle" }
      : matchCount === 0
        ? { kind: "empty" }
        : { kind: "matched", position: index + 1, total: matchCount, more: hasMore };

  return {
    open,
    query,
    status,
    setQuery,
    next: useCallback(() => step(1), [step]),
    previous: useCallback(() => step(-1), [step]),
    close,
    hits,
    hasMore,
    loadingMore,
    loadMore,
    selectedIndex: index,
    select,
    openAt,
    listOpen,
    setListOpen,
  };
}

/** Union of two newest-first match lists, by item, kept newest first. */
function mergeHits(
  first: readonly ConversationSearchHit[],
  second: readonly ConversationSearchHit[],
): ConversationSearchHit[] {
  const seen = new Set<string>();
  const merged: ConversationSearchHit[] = [];
  for (const hit of [...first, ...second]) {
    if (seen.has(hit.itemId)) continue;
    seen.add(hit.itemId);
    merged.push(hit);
  }
  return merged.sort((left, right) => right.startSeq - left.startSeq);
}
