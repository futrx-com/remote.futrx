import type { UsageTotals } from "../../models/usage.ts";

// How usage numbers are written for people — the tokens column, the money
// column, and the caveat that goes with money the platform had to estimate.
// Read across the usage tables, the KPI tiles, the project line and the chart.
class UsageFormatService {
  /** Compact token counts: 1.2M / 34.5K / 812. */
  tokens(tokens: number): string {
    if (!Number.isFinite(tokens) || tokens <= 0) return "0";
    if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(tokens >= 10_000_000 ? 0 : 1)}M`;
    if (tokens >= 1_000) return `${(tokens / 1_000).toFixed(tokens >= 10_000 ? 0 : 1)}K`;
    return String(Math.round(tokens));
  }

  /** Money is shown with enough precision to be useful at agent-run scale:
   *  sub-cent amounts keep four decimals so a $0.0007 turn is not "$0.00". */
  usd(cost: number): string {
    if (!Number.isFinite(cost) || cost === 0) return "$0.00";
    if (Math.abs(cost) < 0.01) return `$${cost.toFixed(4)}`;
    return `$${cost.toFixed(2)}`;
  }

  /** Cost with its confidence: an all-estimated total is prefixed with `~`, and
   *  runs the platform could not price at all are called out separately. */
  costWithConfidence(totals: UsageTotals): string {
    const cost = this.usd(totals.costUsd);
    if (totals.costUsd > 0 && totals.estimatedCostUsd >= totals.costUsd) return `~${cost}`;
    if (totals.estimatedCostUsd > 0) return `${cost}*`;
    return cost;
  }

  /** Human note about how much of a total is estimated or missing. */
  confidenceNote(totals: UsageTotals): string | null {
    const notes: string[] = [];
    if (totals.estimatedCostUsd > 0) {
      notes.push(`${this.usd(totals.estimatedCostUsd)} estimated from the price table`);
    }
    if (totals.unpricedRuns > 0) {
      notes.push(
        `${totals.unpricedRuns} run${totals.unpricedRuns === 1 ? "" : "s"} with unknown cost`
      );
    }
    return notes.length > 0 ? notes.join(" · ") : null;
  }
}

export const usageFormatService = new UsageFormatService();
