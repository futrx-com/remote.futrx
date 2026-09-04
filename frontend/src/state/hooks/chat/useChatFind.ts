import { useCallback, useEffect, useLayoutEffect, useState } from "preact/hooks";
import type { RefObject } from "preact";
import { CHAT_FIND_SKIP_SELECTOR } from "../../../config/chat.ts";
import { isFindShortcut } from "../../../config/shortcuts.ts";
import { chatFindHighlightService } from "../../../services/chat/chatFindHighlightService.ts";
import { domTextSearchService } from "../../../services/platform/domTextSearchService.ts";
import { useDismissShortcut } from "../shared/useDismissShortcut.ts";
import { useShortcut } from "../shared/useShortcut.ts";

/**
 * Where the search stands. A union rather than a loose `(index, matchCount)`
 * pair, because only these three combinations are real: the bar was showing
 * one of three things and had to re-derive which from three separate fields,
 * two of which could disagree.
 */
export type FindStatus =
  | { kind: "idle" }
  | { kind: "empty" }
  | { kind: "matched"; position: number; total: number };

export interface ChatFind {
  open: boolean;
  query: string;
  status: FindStatus;
  setQuery: (query: string) => void;
  next: () => void;
  previous: () => void;
  close: () => void;
}

/**
 * Find-in-chat: the query, and where you are in the results.
 *
 * Matches come from the rendered thread rather than the message model, so what
 * it counts is exactly what is on screen -- see `domTextSearchService`.
 * `revision` is any value that changes when the thread's content does, so a
 * match list cannot go stale against a streaming reply.
 */
export function useChatFind({
  scrollRef,
  contentRef,
  revision,
}: {
  scrollRef: RefObject<HTMLDivElement>;
  contentRef: RefObject<HTMLDivElement>;
  revision: number;
}): ChatFind {
  const [open, setOpen] = useState(false);
  const [query, setQueryState] = useState("");
  const [index, setIndex] = useState(0);
  const [matchCount, setMatchCount] = useState(0);

  const close = useCallback(() => setOpen(false), []);

  useShortcut(isFindShortcut, (event) => {
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
      setMatchCount(0);
      chatFindHighlightService.clear();
      return;
    }
    const ranges = domTextSearchService.findRanges(
      contentRef.current,
      query,
      CHAT_FIND_SKIP_SELECTOR
    );
    setMatchCount(ranges.length);
    if (ranges.length === 0) {
      chatFindHighlightService.clear();
      return;
    }
    // Content can shrink under a held cursor (a message collapsing, older
    // messages dropping out), so the index is clamped rather than trusted.
    if (index >= ranges.length) {
      setIndex(0);
      return;
    }
    chatFindHighlightService.show(ranges, ranges[index]);
    chatFindHighlightService.reveal(scrollRef.current, ranges[index]);
  }, [open, query, index, revision, contentRef, scrollRef]);

  useEffect(() => () => chatFindHighlightService.clear(), []);

  const step = useCallback(
    (delta: number) =>
      setIndex((current) => (matchCount === 0 ? 0 : (current + delta + matchCount) % matchCount)),
    [matchCount]
  );

  const status: FindStatus =
    query.trim().length === 0
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
  };
}
