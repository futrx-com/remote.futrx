import { useCallback, useEffect, useMemo, useRef, useState } from "preact/hooks";
import { chatApi } from "../../../api/chatApi";
import type { ChatSearchHit } from "../../../api/chat/chatTranscriptApi";
import type { WorkspaceSearchSelection } from "../../../api/chat/workspaceSearchApi";
import { CHAT_SEARCH_PAGE_LIMIT } from "../../../config/api.ts";
import { WORKSPACE_SEARCH_DEBOUNCE_MS } from "../../../config/search.ts";

export interface MessageSearch {
  hits: ChatSearchHit[];
  /** The first page is on its way. */
  loading: boolean;
  hasMore: boolean;
  loadingMore: boolean;
  error: string | null;
  loadMore: () => void;
}

interface SearchRequest {
  query: string;
  selection: WorkspaceSearchSelection;
}
interface SearchPage {
  request: SearchRequest;
  hits: ChatSearchHit[];
  hasMore: boolean;
  error: string | null;
}

/** Search transcripts across chats matching the sidebar's filters. */
export function useMessageSearch(query: string, selection: WorkspaceSearchSelection): MessageSearch {
  const trimmed = query.trim();
  const request = useMemo(() => ({ query: trimmed, selection }), [trimmed, selection]);
  const requestRef = useRef(request);
  requestRef.current = request;
  const [page, setPage] = useState<SearchPage | null>(null);
  const current = page?.request === request ? page : null;
  const pendingPage = useRef<SearchRequest | null>(null);
  const [paging, setPaging] = useState<SearchRequest | null>(null);

  useEffect(() => {
    if (!request.query) return;
    let cancelled = false;
    const timer = setTimeout(() => {
      chatApi.searchChats(request.query, undefined, request.selection).then((hits) => {
        if (!cancelled) setPage({ request, hits, hasMore: hits.length >= CHAT_SEARCH_PAGE_LIMIT, error: null });
      }).catch((error: Error) => {
        if (!cancelled) setPage({ request, hits: [], hasMore: false, error: error.message });
      });
    }, WORKSPACE_SEARCH_DEBOUNCE_MS);
    return () => { cancelled = true; clearTimeout(timer); };
  }, [request]);

  const loadMore = useCallback(() => {
    if (!current?.hasMore || pendingPage.current === request) return;
    const last = current.hits.at(-1);
    if (!last) return;
    pendingPage.current = request;
    setPaging(request);
    chatApi.searchChats(request.query, last, request.selection).then((hits) => {
      if (requestRef.current !== request) return;
      setPage({ request, hits: [...current.hits, ...hits], hasMore: hits.length >= CHAT_SEARCH_PAGE_LIMIT, error: null });
    }).catch((error: Error) => {
      if (requestRef.current === request) setPage({ ...current, error: error.message });
    }).finally(() => {
      if (pendingPage.current === request) pendingPage.current = null;
      setPaging((active) => active === request ? null : active);
    });
  }, [current, request]);

  return {
    hits: current?.hits ?? [],
    loading: !!trimmed && !current,
    hasMore: current?.hasMore ?? false,
    loadingMore: paging === request,
    error: current?.error ?? null,
    loadMore,
  };
}
