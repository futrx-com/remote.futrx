import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { chatApi } from "../../../api/chatApi";
import type { ChatSearchHit } from "../../../api/chat/chatTranscriptApi";
import { CHAT_SEARCH_PAGE_LIMIT } from "../../../config/api.ts";

export interface MessageSearch {
  hits: ChatSearchHit[];
  /** The first page is on its way. */
  loading: boolean;
  hasMore: boolean;
  loadingMore: boolean;
  error: string | null;
  loadMore: () => void;
}

/** Typing settles for this long before the server is asked. */
const MESSAGE_SEARCH_DEBOUNCE_MS = 300;

/**
 * Message content across every chat, for `query`, newest first and paged on
 * request. A new query discards the previous one's results.
 */
export function useMessageSearch(query: string): MessageSearch {
  const [hits, setHits] = useState<ChatSearchHit[]>([]);
  const [loading, setLoading] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const trimmed = query.trim();
  const queryRef = useRef(trimmed);
  queryRef.current = trimmed;

  useEffect(() => {
    let cancelled = false;
    setHits([]);
    setHasMore(false);
    setError(null);
    if (!trimmed) { setLoading(false); return; }
    setLoading(true);
    const timer = setTimeout(() => {
      chatApi.searchChats(trimmed).then((page) => {
        if (cancelled) return;
        setHits(page);
        setHasMore(page.length >= CHAT_SEARCH_PAGE_LIMIT);
      }).catch((reason: Error) => {
        if (!cancelled) setError(reason.message);
      }).finally(() => {
        if (!cancelled) setLoading(false);
      });
    }, MESSAGE_SEARCH_DEBOUNCE_MS);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [trimmed]);

  const loadMore = useCallback(() => {
    const last = hits[hits.length - 1];
    if (!last || !hasMore || loadingMore) return;
    const forQuery = trimmed;
    setLoadingMore(true);
    chatApi.searchChats(forQuery, last).then((page) => {
      if (queryRef.current !== forQuery) return;
      setHits((current) => [...current, ...page]);
      setHasMore(page.length >= CHAT_SEARCH_PAGE_LIMIT);
    }).catch((reason: Error) => {
      if (queryRef.current === forQuery) setError(reason.message);
    }).finally(() => setLoadingMore(false));
  }, [hasMore, hits, loadingMore, trimmed]);

  return { hits, loading, hasMore, loadingMore, error, loadMore };
}
