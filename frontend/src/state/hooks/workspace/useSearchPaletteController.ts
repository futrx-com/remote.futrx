import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import { MAX_PALETTE_RESULTS } from "../../../config/search.ts";
import { commandPaletteKeyService } from "../../../services/workspace/commandPaletteKeyService.ts";
import type { WorkspaceSearch } from "./useWorkspaceSearch.ts";

/** Adapt search results and keyboard decisions to the palette's local interaction. */
export function useSearchPaletteController({
  search,
  open,
  onClose,
  onSelectChat,
}: {
  search: WorkspaceSearch;
  open: boolean;
  onClose: () => void;
  onSelectChat: (chatId: string) => void;
}) {
  const [activeIndex, setActiveIndex] = useState(0);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  const results = useMemo(
    () => search.outcome.hits.slice(0, MAX_PALETTE_RESULTS),
    [search.outcome]
  );

  useEffect(() => {
    if (!open) {
      // A reopened palette starts on the results, never on a stale filter menu.
      setFiltersOpen(false);
      return;
    }
    const id = window.setTimeout(() => {
      inputRef.current?.focus();
      inputRef.current?.select();
    }, 0);
    return () => window.clearTimeout(id);
  }, [open]);

  // Any change to the result set invalidates the previous cursor position.
  useEffect(() => setActiveIndex(0), [search.query, search.filters, open]);

  // Keep the highlighted row in view during keyboard navigation.
  useEffect(() => {
    if (!open || filtersOpen) return;
    const list = listRef.current;
    const active = list?.querySelector<HTMLElement>('[data-active="true"]');
    active?.scrollIntoView({ block: "nearest" });
  }, [activeIndex, open, filtersOpen]);

  function closeFilters() {
    setFiltersOpen(false);
    inputRef.current?.focus();
  }

  function choose(index: number) {
    const hit = results[index];
    if (!hit) return;
    onSelectChat(hit.doc.chat.id);
    onClose();
  }

  function onKeyDown(event: KeyboardEvent) {
    const action = commandPaletteKeyService.next(event, {
      activeIndex,
      resultCount: results.length,
      filtersOpen,
    });
    if (action.kind === "ignore") return;

    event.preventDefault();
    // The palette answers for this press, so nothing behind it -- the sidebar,
    // a streaming reply -- sees the same key.
    event.stopPropagation();

    switch (action.kind) {
      case "highlight":
        setActiveIndex(action.index);
        break;
      case "open":
        choose(activeIndex);
        break;
      case "closeFilters":
        closeFilters();
        break;
      case "close":
        onClose();
        break;
    }
  }

  return {
    activeIndex,
    filtersOpen,
    inputRef,
    listRef,
    results,
    closeFilters,
    choose,
    onKeyDown,
    activateResult: (index: number) => setActiveIndex(index),
    toggleFilters: () => (filtersOpen ? closeFilters() : setFiltersOpen(true)),
  };
}
