import { useEffect, useRef, useState } from "preact/hooks";
import type { FacetView, SortId } from "../../models/search";
import type { FilterControl } from "../../state/hooks/workspace/useWorkspaceSearch";
import { SORT_OPTIONS } from "../../config/search";
import { DateRangeControl } from "./DateRangeControl";
import { FilterSection } from "./FilterSection";
import { MultiSelectList } from "./MultiSelectList";
import { X } from "../primitives/icons";

/** One facet rendered as a collapsible checkbox list. */
function FacetFilterSection({
  facet,
  expanded,
  onToggleSection,
  search,
}: {
  facet: FacetView;
  expanded: boolean;
  onToggleSection: () => void;
  search: FilterControl;
}) {
  return (
    <FilterSection
      label={facet.label}
      expanded={expanded}
      selectedCount={facet.selected.length}
      onToggle={onToggleSection}
      onClear={() => search.clearFacet(facet.id)}
    >
      <MultiSelectList
        options={facet.options}
        selected={facet.selected}
        counts={facet.counts}
        emptyHint={facet.emptyHint}
        filterPlaceholder={`Filter ${facet.label.toLowerCase()}`}
        onToggle={(value) => search.toggleFacetValue(facet.id, value)}
        onSetAll={(values) => search.setFacetValues(facet.id, values)}
      />
    </FilterSection>
  );
}

/**
 * The filter menu's contents. Every section except the date range is generated
 * from the facet registry, so adding a filter there makes it appear here
 * automatically.
 *
 * It takes its height from its parent so the same body serves the sidebar's
 * floating menu and the palette, which embeds it in place of the result list.
 */
export function FilterPanelBody({
  search,
  resultCount,
  onClose,
}: {
  search: FilterControl;
  resultCount: number;
  onClose: () => void;
}) {
  // Keyed by section: every facet id, plus "date" and "advanced" for the two
  // sections that aren't facets. Facet ids never collide with those two.
  const [expanded, setExpanded] = useState<Record<string, boolean>>({
    project: true,
    date: true,
  });
  const panelRef = useRef<HTMLDivElement>(null);

  // Counts are only worth computing while this menu is on screen. The retain
  // releases on unmount, so a second menu's counts outlive this one closing.
  const { retainCounts } = search;
  useEffect(() => retainCounts(), [retainCounts]);

  // Move focus into the panel so keyboard and screen-reader users land here.
  useEffect(() => {
    const id = window.setTimeout(() => panelRef.current?.focus(), 0);
    return () => window.clearTimeout(id);
  }, []);

  function toggleSection(id: string) {
    setExpanded((current) => ({ ...current, [id]: !current[id] }));
  }

  const basicFacets = search.facetViews.filter((facet) => !facet.advanced);
  const advancedFacets = search.facetViews.filter((facet) => facet.advanced);

  return (
    <div
      ref={panelRef}
      tabIndex={-1}
      role="dialog"
      aria-label="Search filters"
      class="flex h-full min-h-0 flex-col overflow-hidden focus:outline-none"
    >
      <header class="flex flex-none items-center gap-2 border-b border-line px-3 py-2">
        <span class="text-[12px] font-semibold text-ink-50">Filters</span>
        {search.hasActiveFilters && (
          <button
            type="button"
            onClick={search.resetFilters}
            class="rounded px-1.5 py-0.5 text-[11px] text-ink-300 hover:bg-tint-strong hover:text-ink-50 transition-colors"
          >
            Reset all
          </button>
        )}
        <button
          type="button"
          onClick={onClose}
          class="ml-auto grid h-6 w-6 flex-none place-items-center rounded text-ink-300 hover:bg-tint-strong hover:text-ink-50 transition-colors"
          aria-label="Close filters"
        >
          <X class="h-3.5 w-3.5" />
        </button>
      </header>

      <div class="min-h-0 flex-1 overflow-y-auto touch-scroll scrollbar-thin px-1 py-1">
        {basicFacets.map((facet) => (
          <FacetFilterSection
            key={facet.id}
            facet={facet}
            expanded={expanded[facet.id] === true}
            onToggleSection={() => toggleSection(facet.id)}
            search={search}
          />
        ))}

        <FilterSection
          label="Date"
          expanded={expanded.date === true}
          selectedCount={search.dateView.active ? 1 : 0}
          onToggle={() => toggleSection("date")}
          onClear={search.clearDate}
        >
          <DateRangeControl value={search.filters.date} onChange={search.setDateFilter} />
        </FilterSection>

        <FilterSection
          label="Advanced"
          expanded={expanded.advanced === true}
          selectedCount={advancedFacets.reduce((total, facet) => total + facet.selected.length, 0)}
          onToggle={() => toggleSection("advanced")}
        >
          <div class="space-y-1">
            {advancedFacets.map((facet) => (
              <FacetFilterSection
                key={facet.id}
                facet={facet}
                expanded={expanded[facet.id] === true}
                onToggleSection={() => toggleSection(facet.id)}
                search={search}
              />
            ))}
          </div>
        </FilterSection>
      </div>

      <footer class="flex flex-none items-center gap-2 border-t border-line px-2.5 py-2">
        <span class="text-[11.5px] text-ink-300">
          {resultCount} chat{resultCount === 1 ? "" : "s"}
        </span>
        <label class="ml-auto flex items-center gap-1.5 text-[11px] text-ink-400">
          Sort
          <select
            value={search.sort}
            onChange={(event) =>
              search.setSort((event.currentTarget as HTMLSelectElement).value as SortId)
            }
            class="rounded-control border border-line bg-inset px-1.5 py-1 text-[11px] text-ink-100
                   focus:outline-none focus:border-accent-blue/60"
          >
            {SORT_OPTIONS.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
      </footer>
    </div>
  );
}

/** The sidebar's floating filter menu: the body in a popover shell. */
export function FilterPanel(props: {
  search: FilterControl;
  resultCount: number;
  onClose: () => void;
}) {
  return (
    <div
      class="theme-menu-surface flex max-h-[min(70vh,32rem)] flex-col overflow-hidden rounded-card
             border border-line bg-raised shadow-pop"
    >
      <FilterPanelBody {...props} />
    </div>
  );
}
