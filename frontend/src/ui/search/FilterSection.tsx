import type { ComponentChildren } from "preact";
import { ChevronDown, ChevronRight } from "../primitives/icons";

/** One collapsible group in the filter menu. */
export function FilterSection({
  label,
  expanded,
  selectedCount,
  onToggle,
  onClear,
  children,
}: {
  label: string;
  expanded: boolean;
  selectedCount: number;
  onToggle: () => void;
  onClear?: () => void;
  children: ComponentChildren;
}) {
  const sectionId = `search-filter-${label.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`;

  return (
    <section class="border-b border-line last:border-b-0">
      <div class="flex items-center gap-1">
        <button
          type="button"
          onClick={onToggle}
          class="flex-1 min-w-0 flex items-center gap-1.5 px-2.5 py-2 text-left rounded-control
                 text-[12px] font-semibold text-ink-100 hover:bg-tint transition-colors"
          aria-expanded={expanded}
          aria-controls={sectionId}
        >
          {expanded ? (
            <ChevronDown class="w-3.5 h-3.5 flex-none text-ink-400" />
          ) : (
            <ChevronRight class="w-3.5 h-3.5 flex-none text-ink-400" />
          )}
          <span class="truncate">{label}</span>
          {selectedCount > 0 && (
            <span class="ml-auto flex-none rounded-full bg-accent-blue/[0.16] px-1.5 py-0.5 text-[10px] font-semibold leading-none text-accent-blue">
              {selectedCount}
            </span>
          )}
        </button>
        {selectedCount > 0 && onClear && (
          <button
            type="button"
            onClick={onClear}
            class="flex-none mr-1.5 rounded px-1.5 py-1 text-[10.5px] text-ink-400 hover:text-ink-100 hover:bg-tint-strong transition-colors"
            title={`Clear ${label.toLowerCase()}`}
          >
            Clear
          </button>
        )}
      </div>
      {expanded && (
        <div id={sectionId} class="px-1.5 pb-2">
          {children}
        </div>
      )}
    </section>
  );
}
