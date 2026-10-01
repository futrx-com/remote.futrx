import { useCallback, useEffect, useState } from "preact/hooks";
import type { ChatFind } from "./useChatFind.ts";
import { terminalFindState, type TerminalSearchResults } from "./terminalFindState.ts";
import type { TerminalSearch } from "./useTerminalSession.ts";
import { useDismissShortcut } from "../shared/useDismissShortcut.ts";

export interface TerminalFind extends ChatFind {
  show: () => void;
}

/**
 * Find-in-terminal: the same bar and keys as find-in-chat, searching the
 * terminal's buffer through its search addon instead of the rendered thread.
 */
export function useTerminalFind({
  search,
  results,
  onClose,
}: {
  search: TerminalSearch;
  results: TerminalSearchResults | null;
  /** Called after the bar closes, so focus can go back to the shell. */
  onClose: () => void;
}): TerminalFind {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  // Typing extends the current match rather than jumping past it.
  useEffect(() => {
    if (!open) return;
    if (query) search.next(query, true);
    else search.clear();
  }, [open, query, search]);

  const close = useCallback(() => {
    setOpen(false);
    search.clear();
    onClose();
  }, [search, onClose]);

  useDismissShortcut(
    (event) => {
      // The bar's input usually has focus, where Escape can revert the query first.
      event.preventDefault();
      close();
    },
    { enabled: open },
  );

  return {
    open,
    query,
    status: terminalFindState.status(query, results),
    setQuery,
    next: useCallback(() => {
      if (query) search.next(query);
    }, [query, search]),
    previous: useCallback(() => {
      if (query) search.previous(query);
    }, [query, search]),
    close,
    show: useCallback(() => setOpen(true), []),
  };
}
