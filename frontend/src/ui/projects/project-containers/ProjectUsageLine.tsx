import type { UsageSummary } from "../../../models/usage";
import { usageFormatService } from "../../../services/usage/usageFormatService.ts";
import { Activity } from "../../primitives/icons";

/**
 * Month-to-date spend for one project, shown under the project header. It is
 * deliberately one line: the full breakdown lives on Settings → Usage.
 */
export function ProjectUsageLine({
  summary,
  loading,
  error,
}: {
  summary: UsageSummary | null;
  loading: boolean;
  error: string | null;
}) {
  if (error) return null;

  const totals = summary?.totals;
  const detail = loading && !summary
    ? "Loading…"
    : !totals || totals.runs === 0
      ? "No agent runs yet this month"
      : `${usageFormatService.costWithConfidence(totals)} · ${usageFormatService.tokens(totals.totalTokens)} tokens · ${
          totals.runs
        } run${totals.runs === 1 ? "" : "s"}`;

  return (
    <div class="mt-2 inline-flex items-center gap-2 text-[12px] text-ink-300">
      <Activity class="w-3.5 h-3.5 text-accent-green flex-none" />
      <span class="text-ink-400">This month:</span>
      <span class="text-ink-200">{detail}</span>
      {totals && totals.unpricedRuns > 0 && (
        <span
          class="text-ink-400"
          title={`${totals.unpricedRuns} run${
            totals.unpricedRuns === 1 ? "" : "s"
          } could not be priced — the provider reported no cost and no price-table entry matched`}
        >
          ({totals.unpricedRuns} unpriced)
        </span>
      )}
    </div>
  );
}
