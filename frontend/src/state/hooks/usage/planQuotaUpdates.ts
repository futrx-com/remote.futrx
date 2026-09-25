import {
  PLAN_QUOTA_CLOCK_INTERVAL_MS,
  PLAN_QUOTA_POLL_INTERVAL_MS,
  PLAN_QUOTA_REQUEST_TIMEOUT_MS,
} from "../../../config/planQuota.ts";
import type { AccountQuota } from "../../../models/agentQuota.ts";

/** One mounted quota section owns its request, refresh timer and display clock. */
export function startPlanQuotaUpdates({
  load,
  onSnapshot,
  onSettled,
  onClock,
}: {
  load: (signal: AbortSignal) => Promise<AccountQuota[]>;
  onSnapshot: (quotas: AccountQuota[]) => void;
  onSettled: () => void;
  onClock: (nowMs: number) => void;
}): () => void {
  let disposed = false;
  let refreshTimer: ReturnType<typeof setTimeout> | undefined;
  let deadlineTimer: ReturnType<typeof setTimeout> | undefined;
  let activeRequest: AbortController | undefined;

  const clockTimer = setInterval(() => onClock(Date.now()), PLAN_QUOTA_CLOCK_INTERVAL_MS);

  async function refresh() {
    const request = new AbortController();
    activeRequest = request;
    deadlineTimer = setTimeout(() => request.abort(), PLAN_QUOTA_REQUEST_TIMEOUT_MS);
    try {
      const quotas = await load(request.signal);
      if (!disposed && !request.signal.aborted) onSnapshot(quotas);
    } catch {
      // A missed refresh must not erase the last successful account reading.
    } finally {
      clearTimeout(deadlineTimer);
      activeRequest = undefined;
      if (!disposed) {
        onSettled();
        onClock(Date.now());
        refreshTimer = setTimeout(refresh, PLAN_QUOTA_POLL_INTERVAL_MS);
      }
    }
  }

  void refresh();
  return () => {
    disposed = true;
    clearInterval(clockTimer);
    clearTimeout(refreshTimer);
    clearTimeout(deadlineTimer);
    activeRequest?.abort();
  };
}
