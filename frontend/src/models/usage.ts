export type UsageGroupBy = "project" | "user" | "provider" | "model" | "day" | "chat";

export interface UsageTotals {
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  totalTokens: number;
  /** Sum of every run whose cost is known, exact or estimated. */
  costUsd: number;
  /** The part of costUsd that came from the editable price table. */
  estimatedCostUsd: number;
  runs: number;
  /** Runs whose cost could not be determined at all. */
  unpricedRuns: number;
}

export interface UsageGroup extends UsageTotals {
  key: string;
  label: string;
}

export interface UsageDayPoint {
  day: string;
  totalTokens: number;
  costUsd: number;
  runs: number;
}

export interface UsageSummary {
  from: number;
  to: number;
  groupBy: UsageGroupBy;
  totals: UsageTotals;
  projects: number;
  groups: UsageGroup[];
  daily: UsageDayPoint[];
}

export interface UsageRecord {
  at: number;
  projectId?: string;
  projectSlug?: string;
  chatId: string;
  runId?: string;
  userEmail?: string;
  provider: string;
  model?: string;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  /** Absent when the run's cost is unknown — never treat that as zero. */
  costUsd?: number;
  estimated?: boolean;
  durationMs?: number;
  turns?: number;
  scheduled?: boolean;
}

export interface UsageRecordPage {
  records: UsageRecord[];
  nextCursor?: string;
}

export interface UsageModelPrice {
  match: string;
  label?: string;
  inputPerMTok: number;
  outputPerMTok: number;
  cacheReadPerMTok?: number;
  cacheWritePerMTok?: number;
}

export interface UsagePriceTable {
  version: number;
  updatedAt?: number;
  currency: string;
  models: UsageModelPrice[];
}

export interface UsageRebuildResult {
  chats: number;
  records: number;
  months: string[];
  preservedActors: number;
}

export const EMPTY_USAGE_TOTALS: UsageTotals = {
  inputTokens: 0,
  outputTokens: 0,
  cacheReadTokens: 0,
  cacheWriteTokens: 0,
  totalTokens: 0,
  costUsd: 0,
  estimatedCostUsd: 0,
  runs: 0,
  unpricedRuns: 0,
};

/**
 * The Usage page's selected window. Every range resolves to a half-open
 * millisecond window the backend can filter on; days are bounded in UTC
 * because the ledger buckets records by UTC day.
 */
export type UsageRangePreset = "7d" | "30d" | "month" | "custom";

export interface UsageRange {
  preset: UsageRangePreset;
  from: number;
  to: number;
}

/** A range as the two `<input type=date>` values that show it. */
export interface UsageRangeLabels {
  fromDate: string;
  toDate: string;
}

/** The per-project record list opened from a summary row. */
export interface UsageDrillDown {
  projectId: string;
  label: string;
  records: UsageRecord[];
  loading: boolean;
  error: string | null;
  hasMore: boolean;
}

export type UsageChartMetric = "tokens" | "cost";

export interface UsageChartBar {
  day: string;
  value: number;
  /** Fraction of the tallest bar, 0..1. */
  ratio: number;
  runs: number;
  label: string;
}

export interface UsageChartModel {
  bars: UsageChartBar[];
  peak: number;
  peakLabel: string;
  /** True when every day in the window is empty, so the view can say so. */
  isEmpty: boolean;
}
