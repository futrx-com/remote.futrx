import { useEffect, useState } from "preact/hooks";
import { agentQuotaApi } from "../../../api/agents/agentQuotaApi";
import type { AccountQuota } from "../../../models/agentQuota";
import type { PlanQuotaProvider } from "../../../models/planQuota";
import { useAuthContext } from "../../context/AuthContext";
import { projectPlanQuota } from "./planQuotaState";
import { startPlanQuotaUpdates } from "./planQuotaUpdates";

export interface PlanQuotaState {
  providers: PlanQuotaProvider[];
  loading: boolean;
}

/** Reads stored snapshots while the Usage tab is mounted, ages each reading,
 *  and attributes it to a saved account from the agent-auth catalog. */
export function usePlanQuota(): PlanQuotaState {
  const { agentAuth } = useAuthContext();
  const [quotas, setQuotas] = useState<AccountQuota[] | null>(null);
  const [loading, setLoading] = useState(true);
  const [nowMs, setNowMs] = useState(Date.now);

  useEffect(() => startPlanQuotaUpdates({
    load: (signal) => agentQuotaApi.list(signal),
    onSnapshot: setQuotas,
    onSettled: () => setLoading(false),
    onClock: setNowMs,
  }), []);

  return {
    providers: projectPlanQuota(quotas ?? [], agentAuth.providers, nowMs),
    loading,
  };
}
