import { DATE_FIELD_OPTIONS, DATE_PRESET_OPTIONS } from "../../config/search";
import type { DateFilter, DatePresetId } from "../../models/search";

/**
 * Date presets plus a custom range, and a choice of which timestamp to test.
 * "Last activity" is the default because it is what the sidebar already sorts
 * by; "Created" answers a different question ("when did I start this?").
 */
export function DateRangeControl({
  value,
  onChange,
}: {
  value: DateFilter;
  onChange: (next: DateFilter) => void;
}) {
  function choosePreset(preset: DatePresetId) {
    // Leaving custom drops the dates so a stale range can't linger invisibly.
    if (preset === "custom") onChange({ ...value, preset });
    else onChange({ preset, field: value.field });
  }

  return (
    <div class="space-y-2">
      <div class="flex flex-wrap gap-1">
        {DATE_PRESET_OPTIONS.map((option) => {
          const active = value.preset === option.value;
          return (
            <button
              key={option.value}
              type="button"
              onClick={() => choosePreset(option.value)}
              class={`rounded-control px-2 py-1 text-[11px] font-medium transition-colors
                      ${active
                        ? "bg-accent-blue/[0.16] text-accent-blue"
                        : "bg-tint text-ink-200 hover:bg-tint-strong hover:text-ink-100"}`}
              aria-pressed={active}
            >
              {option.label}
            </button>
          );
        })}
      </div>

      {value.preset === "custom" && (
        <div class="flex items-center gap-1.5">
          <input
            type="date"
            value={value.from ?? ""}
            onInput={(event) =>
              onChange({ ...value, from: (event.currentTarget as HTMLInputElement).value })
            }
            class="min-w-0 flex-1 h-8 rounded-control bg-inset border border-line px-2
                   text-[11.5px] text-ink-100 focus:outline-none focus:border-accent-blue/60"
            aria-label="From date"
          />
          <span class="flex-none text-[11px] text-ink-400">to</span>
          <input
            type="date"
            value={value.to ?? ""}
            onInput={(event) =>
              onChange({ ...value, to: (event.currentTarget as HTMLInputElement).value })
            }
            class="min-w-0 flex-1 h-8 rounded-control bg-inset border border-line px-2
                   text-[11.5px] text-ink-100 focus:outline-none focus:border-accent-blue/60"
            aria-label="To date"
          />
        </div>
      )}

      <div
        class="flex gap-1 rounded-control bg-tint p-0.5"
        role="radiogroup"
        aria-label="Date field"
      >
        {DATE_FIELD_OPTIONS.map((option) => {
          const active = value.field === option.value;
          return (
            <button
              key={option.value}
              type="button"
              role="radio"
              aria-checked={active}
              onClick={() => onChange({ ...value, field: option.value })}
              class={`flex-1 rounded px-2 py-1 text-[11px] font-medium transition-colors
                      ${active ? "bg-accent-blue/[0.18] text-accent-blue" : "text-ink-300 hover:text-ink-100"}`}
            >
              {option.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}
