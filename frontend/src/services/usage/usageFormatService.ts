// How usage numbers are written for people. Read across the usage tables, the
// KPI tiles, the project line and the chart.
class UsageFormatService {
  /** Compact token counts: 1.2M / 34.5K / 812. */
  tokens(tokens: number): string {
    if (!Number.isFinite(tokens) || tokens <= 0) return "0";
    if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(tokens >= 10_000_000 ? 0 : 1)}M`;
    if (tokens >= 1_000) return `${(tokens / 1_000).toFixed(tokens >= 10_000 ? 0 : 1)}K`;
    return String(Math.round(tokens));
  }
}

export const usageFormatService = new UsageFormatService();
