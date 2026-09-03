import { requestJson } from "../apiRequest.ts";
import { API_ROUTES } from "../../config/routes.ts";
import type { AgentCapabilitiesCatalog } from "../../models/agentCapabilities";

export const capabilitiesApi = {
  list: (projectId?: string, options: { refresh?: boolean } = {}) => {
    const params = new URLSearchParams();
    if (projectId) params.set("projectId", projectId);
    if (options.refresh) params.set("refresh", "1");
    return requestJson<AgentCapabilitiesCatalog>(
      "GET",
      API_ROUTES.agentCapabilities(params.toString()),
    );
  },
};
