import { useEffect, useState } from "preact/hooks";
import { agentQuotaApi } from "../../../api/agents/agentQuotaApi";
import type { AgentQuota, PlanQuotaRow } from "../../../models/agentQuota";
import { projectPlanQuotaRows } from "./planQuotaState";

export interface PlanQuotaState {
  rows: PlanQuotaRow[];
  loading: boolean;
}

/** Owns the Usage tab's one-shot subscription-quota request lifecycle. */
export function usePlanQuota(): PlanQuotaState {
  const [quotas, setQuotas] = useState<AgentQuota[] | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    agentQuotaApi
      .list()
      .then((value) => !cancelled && setQuotas(value))
      .catch(() => !cancelled && setQuotas([]))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, []);

  return {
    rows: projectPlanQuotaRows(quotas ?? [], Date.now()),
    loading,
  };
}
