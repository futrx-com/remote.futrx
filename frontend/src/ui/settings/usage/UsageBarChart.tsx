import type { UsageChartMetric } from "../../../models/usage";
import { usageChartService } from "../../../services/usage/usageChartService.ts";
import type { UsageDayPoint } from "../../../models/usage";

const VIEW_WIDTH = 720;
const VIEW_HEIGHT = 120;
const MIN_VISIBLE_RATIO = 0.015;

/**
 * Per-day bar chart drawn as inline SVG — no chart library, no runtime
 * measurement. The viewBox scales the drawing to whatever width the card
 * gets, so the only responsive rule needed is `w-full`.
 */
export function UsageBarChart({
  daily,
  metric,
  onMetricChange,
}: {
  daily: UsageDayPoint[];
  metric: UsageChartMetric;
  onMetricChange: (metric: UsageChartMetric) => void;
}) {
  const chart = usageChartService.build(daily, metric);
  const slot = chart.bars.length > 0 ? VIEW_WIDTH / chart.bars.length : VIEW_WIDTH;
  const barWidth = Math.max(1, slot * 0.62);

  return (
    <section class="rounded-card border border-line bg-surface overflow-hidden">
      <header class="px-4 py-3 border-b border-line flex items-center gap-3">
        <div class="flex-1 min-w-0">
          <div class="text-[14.5px] font-semibold text-ink-50">Daily usage</div>
          <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">
            {chart.isEmpty
              ? "No runs recorded in this range."
              : `Peak day ${chart.peakLabel} · UTC days`}
          </div>
        </div>
        <div class="flex-none inline-flex rounded-md border border-line overflow-hidden">
          {(["tokens", "cost"] as UsageChartMetric[]).map((option) => (
            <button
              key={option}
              type="button"
              onClick={() => onMetricChange(option)}
              aria-pressed={metric === option}
              class={`h-8 px-3 text-[12px] font-medium transition-colors ${
                metric === option
                  ? "bg-tint-active text-ink-50"
                  : "text-ink-300 hover:text-ink-100 hover:bg-tint"
              }`}
            >
              {option === "tokens" ? "Tokens" : "Cost"}
            </button>
          ))}
        </div>
      </header>

      <div class="p-3">
        <svg
          viewBox={`0 0 ${VIEW_WIDTH} ${VIEW_HEIGHT}`}
          preserveAspectRatio="none"
          class="w-full h-[120px] text-ink-400"
          role="img"
          aria-label={`Usage per day, measured in ${metric}`}
        >
          {/* Baseline. currentColor comes from the themed text class on the
              svg, so the axis stays visible in both light and dark. */}
          <line
            x1="0"
            y1={VIEW_HEIGHT - 0.5}
            x2={VIEW_WIDTH}
            y2={VIEW_HEIGHT - 0.5}
            stroke="currentColor"
            stroke-width="1"
            opacity="0.3"
          />
          {chart.bars.map((bar, index) => {
            // Days with no runs draw nothing at all; the baseline already
            // shows where they sit, and an empty rect would need its own
            // per-theme colour to stay visible.
            if (bar.value <= 0) return null;
            // A quiet day still keeps a sliver of height so it reads as
            // "some activity" rather than "none".
            const height = Math.max(bar.ratio, MIN_VISIBLE_RATIO) * (VIEW_HEIGHT - 6);
            return (
              <rect
                key={bar.day}
                x={index * slot + (slot - barWidth) / 2}
                y={VIEW_HEIGHT - height}
                width={barWidth}
                height={height}
                rx="1.5"
                class="fill-accent-blue"
              >
                <title>{bar.label}</title>
              </rect>
            );
          })}
        </svg>
        {chart.bars.length > 0 && (
          <div class="mt-1.5 flex justify-between text-[11px] text-ink-400 font-mono">
            <span>{chart.bars[0].day}</span>
            <span>{chart.bars[chart.bars.length - 1].day}</span>
          </div>
        )}
      </div>
    </section>
  );
}
