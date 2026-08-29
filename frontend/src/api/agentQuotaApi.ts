import { API_ROUTES } from "../config/routes";
import type { AgentQuota } from "../models/agentQuota";

export const agentQuotaApi = {
  async list(): Promise<AgentQuota[]> {
    const response = await fetch(API_ROUTES.agentQuota, { credentials: "same-origin" });
    if (!response.ok) return [];
    const body = (await response.json()) as { agents?: AgentQuota[] } | null;
    return body?.agents ?? [];
  },
};
