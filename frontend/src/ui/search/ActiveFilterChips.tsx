import type { FilterControl } from "../../state/hooks/workspace/useWorkspaceSearch";
import { X } from "../primitives/icons";

/**
 * Active filters shown beneath the input, so the current selection is legible
 * without reopening the menu — and removable in one click.
 */
export function ActiveFilterChips({ search }: { search: FilterControl }) {
  const chips: { key: string; label: string; onRemove: () => void }[] = [];

  for (const facet of search.facetViews) {
    for (const value of facet.selected) {
      const option = facet.options.find((entry) => entry.value === value);
      chips.push({
        key: `${facet.id}:${value}`,
        // A value whose option vanished (a deleted project, say) still needs a
        // chip so the user can see and clear the filter that's hiding results.
        label: option?.label ?? value,
        onRemove: () => search.toggleFacetValue(facet.id, value),
      });
    }
  }

  if (search.dateView.active) {
    chips.push({
      key: "date",
      label: search.dateView.label,
      onRemove: search.clearDate,
    });
  }

  if (chips.length === 0) return null;

  return (
    <div class="mt-2 flex flex-wrap gap-1">
      {chips.map((chip) => (
        <span
          key={chip.key}
          class="inline-flex max-w-full items-center gap-1 rounded-full bg-accent-blue/[0.12]
                 py-0.5 pl-2 pr-1 text-[10.5px] font-medium text-accent-blue"
        >
          <span class="truncate">{chip.label}</span>
          <button
            type="button"
            onClick={chip.onRemove}
            class="grid h-4 w-4 flex-none place-items-center rounded-full hover:bg-accent-blue/25 transition-colors"
            aria-label={`Remove filter ${chip.label}`}
          >
            <X class="h-2.5 w-2.5" />
          </button>
        </span>
      ))}
      {chips.length > 1 && (
        <button
          type="button"
          onClick={search.resetFilters}
          class="rounded-full px-2 py-0.5 text-[10.5px] text-ink-400 hover:bg-tint-strong hover:text-ink-100 transition-colors"
        >
          Clear all
        </button>
      )}
    </div>
  );
}
