import { useMemo, useState } from "preact/hooks";
import { FACET_INLINE_FILTER_THRESHOLD } from "../../config/search";
import type { FacetOption } from "../../models/search";
import { Check, Search } from "../primitives/icons";

/**
 * Checkbox list for one facet. Options carry the number of chats they would
 * match given every *other* active filter, so the count previews the result of
 * ticking the box.
 */
export function MultiSelectList({
  options,
  selected,
  counts,
  emptyHint,
  filterPlaceholder,
  onToggle,
  onSetAll,
}: {
  options: readonly FacetOption[];
  selected: readonly string[];
  counts: Map<string, number>;
  emptyHint: string;
  filterPlaceholder: string;
  onToggle: (value: string) => void;
  onSetAll: (values: string[]) => void;
}) {
  const [filter, setFilter] = useState("");
  const selectedSet = useMemo(() => new Set(selected), [selected]);

  const visible = useMemo(() => {
    const term = filter.trim().toLowerCase();
    if (!term) return options;
    return options.filter(
      (option) =>
        option.label.toLowerCase().includes(term) ||
        (option.hint ?? "").toLowerCase().includes(term)
    );
  }, [options, filter]);

  if (options.length === 0) {
    return <p class="px-2 py-1.5 text-[11.5px] text-ink-400">{emptyHint}</p>;
  }

  return (
    <div>
      {options.length > FACET_INLINE_FILTER_THRESHOLD && (
        <label class="mb-1.5 flex items-center gap-1.5 h-8 rounded-control bg-inset border border-line px-2 focus-within:border-accent-blue/60 transition-colors">
          <Search class="w-3 h-3 flex-none text-ink-400" />
          <input
            value={filter}
            onInput={(event) => setFilter((event.currentTarget as HTMLInputElement).value)}
            placeholder={filterPlaceholder}
            class="min-w-0 flex-1 bg-transparent text-[11.5px] text-ink-100 placeholder:text-ink-400 focus:outline-none"
            autocomplete="off"
            spellcheck={false}
          />
        </label>
      )}

      <div class="flex items-center gap-2 px-1 pb-1 text-[10.5px] text-ink-400">
        <button
          type="button"
          onClick={() => onSetAll(options.map((option) => option.value))}
          class="hover:text-ink-100 transition-colors"
        >
          Select all
        </button>
        <span aria-hidden="true">·</span>
        <button
          type="button"
          onClick={() => onSetAll([])}
          class="hover:text-ink-100 transition-colors"
        >
          None
        </button>
        <span class="ml-auto">
          {selected.length > 0 ? `${selected.length} of ${options.length}` : `${options.length}`}
        </span>
      </div>

      <div class="max-h-52 overflow-y-auto touch-scroll scrollbar-thin space-y-1">
        {visible.length === 0 && (
          <p class="px-2 py-1.5 text-[11.5px] text-ink-400">No matches</p>
        )}
        {visible.map((option) => {
          const checked = selectedSet.has(option.value);
          const count = counts.get(option.value) ?? 0;
          return (
            <label
              key={option.value}
              class={`flex items-center gap-2 rounded-control px-2 py-1.5 cursor-pointer transition-colors
                      ${checked ? "bg-accent-blue/[0.12]" : "hover:bg-tint"}`}
            >
              <input
                type="checkbox"
                checked={checked}
                onChange={() => onToggle(option.value)}
                class="sr-only"
              />
              <span
                aria-hidden="true"
                class={`w-3.5 h-3.5 flex-none grid place-items-center rounded-[4px] border transition-colors
                        ${checked
                          ? "bg-accent-blue border-accent-blue text-on-accent"
                          : "border-line-strong"}`}
              >
                {checked && <Check class="w-2.5 h-2.5" />}
              </span>
              <span class="min-w-0 flex-1">
                <span
                  class={`block truncate text-[12px] leading-tight ${
                    checked ? "text-accent-blue font-medium" : "text-ink-100"
                  }`}
                >
                  {option.label}
                </span>
                {option.hint && (
                  <span class="block truncate text-[10.5px] leading-tight text-ink-400">
                    {option.hint}
                  </span>
                )}
              </span>
              <span
                class={`flex-none text-[10.5px] tabular-nums ${
                  count === 0 ? "text-ink-500" : "text-ink-400"
                }`}
              >
                {count}
              </span>
            </label>
          );
        })}
      </div>
    </div>
  );
}
