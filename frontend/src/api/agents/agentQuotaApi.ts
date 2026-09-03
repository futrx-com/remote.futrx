import { API_ROUTES } from "../../config/routes";
import type { AgentQuota, AgentQuotaResponse } from "../../models/agentQuota";
import { sendHttpRequest } from "../../transport/http";

export const agentQuotaApi = {
  async list(): Promise<AgentQuota[]> {
    const response = await sendHttpRequest("GET", API_ROUTES.agentQuota);
    if (!response.ok) return [];
    const body = (await response.json()) as AgentQuotaResponse | null;
    return body?.agents ?? [];
  },
};
