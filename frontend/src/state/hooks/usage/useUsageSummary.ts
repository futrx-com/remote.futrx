import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { usageApi } from "../../../api/usageApi";
import type { UsageGroupBy, UsageRange, UsageSummary } from "../../../models/usage";

/**
 * The aggregate query for the selected window and grouping.
 *
 * loading and refreshing are distinct on purpose: the first query paints a
 * skeleton, later ones keep the current numbers on screen while the next
 * arrives, so changing the grouping does not blank the page.
 */
export function useUsageSummary(enabled: boolean, range: UsageRange) {
  const [groupBy, setGroupBy] = useState<UsageGroupBy>("project");
  const [summary, setSummary] = useState<UsageSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const hasSummary = useRef(false);

  const refresh = useCallback(async () => {
    if (hasSummary.current) setRefreshing(true);
    else setLoading(true);
    try {
      const next = await usageApi.summary({ from: range.from, to: range.to, groupBy });
      hasSummary.current = true;
      setSummary(next);
      setError(null);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [range.from, range.to, groupBy]);

  useEffect(() => {
    if (!enabled) return;
    void refresh();
  }, [enabled, refresh]);

  return { groupBy, setGroupBy, summary, loading, refreshing, error, refresh };
}
