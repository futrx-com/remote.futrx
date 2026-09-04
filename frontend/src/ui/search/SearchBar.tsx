import { useRef, useState } from "preact/hooks";
import type {
  FilterControl,
  QueryControl,
} from "../../state/hooks/workspace/useWorkspaceSearch";
import { isDismissShortcut } from "../../config/shortcuts.ts";
import { useDismissOnOutside } from "../primitives/popover";
import { ActiveFilterChips } from "./ActiveFilterChips";
import { FilterPanel } from "./FilterPanel";
import { Search, SlidersHorizontal, X } from "../primitives/icons";

/**
 * The sidebar search row: a plain keyword input plus the filter menu trigger.
 *
 * Keeping the input free of query syntax is deliberate — the filters live
 * behind a discoverable button so nobody has to learn a grammar to use them.
 */
export function SearchBar({
  search,
  resultCount,
}: {
  search: QueryControl & FilterControl;
  resultCount: number;
}) {
  const [filtersOpen, setFiltersOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useDismissOnOutside(filtersOpen, () => setFiltersOpen(false), rootRef);

  const showClear = search.query.length > 0;

  return (
    <div ref={rootRef} class="relative mt-2">
      <div class="flex items-center gap-1.5">
        <label class="flex h-8 min-w-0 flex-1 items-center gap-2 rounded-control bg-tint px-2.5 transition-colors
                      focus-within:bg-inset focus-within:ring-1 focus-within:ring-accent-blue/50">
          <Search class="h-3.5 w-3.5 flex-none text-ink-400" />
          <input
            ref={inputRef}
            value={search.query}
            onInput={(event) => search.setQuery((event.currentTarget as HTMLInputElement).value)}
            onKeyDown={(event) => {
              if (!isDismissShortcut(event)) return;
              if (!search.query) return;
              // Clear the query before letting Escape bubble out and close the
              // whole sidebar — one Escape, one obvious effect.
              event.stopPropagation();
              search.setQuery("");
            }}
            placeholder="Search chats and projects"
            class="min-w-0 flex-1 bg-transparent text-[13px] text-ink-100 placeholder:text-ink-400 focus:outline-none"
            autocomplete="off"
            spellcheck={false}
            // An Arabic or Hebrew query reads right-to-left; let the browser
            // pick the direction from what was typed rather than forcing LTR.
            dir="auto"
            aria-label="Search chats and projects"
          />
          {showClear && (
            <button
              type="button"
              onClick={() => {
                search.setQuery("");
                inputRef.current?.focus();
              }}
              class="grid h-5 w-5 flex-none place-items-center rounded text-ink-400 hover:bg-tint-strong hover:text-ink-100 transition-colors"
              aria-label="Clear search"
            >
              <X class="h-3 w-3" />
            </button>
          )}
        </label>

        <button
          type="button"
          onClick={() => setFiltersOpen((open) => !open)}
          class={`relative grid h-8 w-8 flex-none place-items-center rounded-control transition-colors
                  ${filtersOpen || search.hasActiveFilters
                    ? "bg-accent-blue/[0.16] text-accent-blue"
                    : "text-ink-300 hover:bg-tint-strong hover:text-ink-50"}`}
          aria-label="Filters"
          aria-haspopup="dialog"
          aria-expanded={filtersOpen}
          title="Filters"
        >
          <SlidersHorizontal class="h-3.5 w-3.5" />
          {search.activeFilterCount > 0 && (
            <span class="absolute -right-0.5 -top-0.5 grid h-4 min-w-4 place-items-center rounded-full bg-accent-blue px-1 text-[9.5px] font-bold leading-none text-on-accent">
              {search.activeFilterCount}
            </span>
          )}
        </button>
      </div>

      <ActiveFilterChips search={search} />

      {filtersOpen && (
        <div class="absolute left-0 right-0 top-full z-50 mt-1.5">
          <FilterPanel
            search={search}
            resultCount={resultCount}
            onClose={() => setFiltersOpen(false)}
          />
        </div>
      )}
    </div>
  );
}
