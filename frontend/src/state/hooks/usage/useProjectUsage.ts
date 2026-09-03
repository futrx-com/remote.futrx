import { useCallback, useEffect, useState } from "preact/hooks";
import { usageApi } from "../../../api/usageApi";
import type { UsageSummary } from "../../../models/usage";
import { usageRangeService } from "../../../services/usage/usageRangeService.ts";

export interface ProjectUsage {
  summary: UsageSummary | null;
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
}

/**
 * Month-to-date usage for one project, used by the project page header. The
 * window is recomputed on each fetch so a long-lived tab rolls into the new
 * month on its own.
 */
export function useProjectUsage(projectId: string | undefined): ProjectUsage {
  const [summary, setSummary] = useState<UsageSummary | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!projectId) {
      setSummary(null);
      setError(null);
      return;
    }
    setLoading(true);
    try {
      const range = usageRangeService.forPreset("month", Date.now());
      setSummary(await usageApi.projectSummary(projectId, range.from, range.to));
      setError(null);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  return { summary, loading, error, refresh };
}
