import { useMemo } from "preact/hooks";
import type {
  UsageDrillDown,
  UsageGroupBy,
  UsageRange,
  UsageRangePreset,
  UsageSummary,
} from "../../../models/usage";
import { useUsageDrillDown } from "./useUsageDrillDown";
import { useUsageRange } from "./useUsageRange";
import { useUsageSummary } from "./useUsageSummary";

export interface UsageDashboard {
  range: UsageRange;
  groupBy: UsageGroupBy;
  summary: UsageSummary | null;
  loading: boolean;
  refreshing: boolean;
  error: string | null;
  drillDown: UsageDrillDown | null;
  setPreset: (preset: UsageRangePreset) => void;
  setCustomRange: (from: string, to: string) => void;
  setGroupBy: (groupBy: UsageGroupBy) => void;
  refresh: () => Promise<void>;
  openDrillDown: (projectId: string, label: string) => Promise<void>;
  loadMoreDrillDown: () => Promise<void>;
  closeDrillDown: () => void;
}

/**
 * The Usage page's query state, composed from its three independent parts: the
 * selected window, the aggregate query over it, and the per-project drill-down.
 * Every state change re-queries the backend rather than aggregating
 * client-side, so a member's view is filtered where the membership rules live.
 *
 * The window is read by both of the others, which is why it is selected first.
 */
export function useUsageDashboard(enabled: boolean): UsageDashboard {
  const { range, setPreset, setCustomRange } = useUsageRange();
  const { groupBy, setGroupBy, summary, loading, refreshing, error, refresh } =
    useUsageSummary(enabled, range);
  const { drillDown, openDrillDown, loadMoreDrillDown, closeDrillDown } =
    useUsageDrillDown(range);

  return useMemo(
    () => ({
      range,
      groupBy,
      summary,
      loading,
      refreshing,
      error,
      drillDown,
      setPreset,
      setCustomRange,
      setGroupBy,
      refresh,
      openDrillDown,
      loadMoreDrillDown,
      closeDrillDown,
    }),
    [
      range,
      groupBy,
      summary,
      loading,
      refreshing,
      error,
      drillDown,
      setPreset,
      setCustomRange,
      refresh,
      openDrillDown,
      loadMoreDrillDown,
      closeDrillDown,
    ]
  );
}
