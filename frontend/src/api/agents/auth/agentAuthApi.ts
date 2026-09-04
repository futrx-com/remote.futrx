import { requestJson } from "../../apiRequest";
import { API_ROUTES, WEB_SOCKET_ROUTES } from "../../../config/routes";
import type {
  AgentAuthCatalog,
  AgentAuthLoginSnapshot,
  AgentAuthSnapshot,
} from "../../../models/auth";
import { subscribeToJsonMessages } from "../../../transport/jsonMessageSubscription";

interface CodeLoginStart {
  url: string;
  resumed?: boolean;
}

interface DeviceLoginStart {
  active: boolean;
  verificationUri?: string;
  userCode?: string;
  startedAt?: number;
  expiresAt?: number;
  completed?: boolean;
  error?: string;
}

export const agentAuthApi = {
  fetchCatalog: () =>
    requestJson<AgentAuthCatalog>("GET", API_ROUTES.agentAuth.catalog),

  startCodeLogin: (provider: string) =>
    requestJson<CodeLoginStart>(
      "POST",
      API_ROUTES.agentAuth.startCodeLogin(provider),
      {},
    ),

  submitCode: (provider: string, code: string) =>
    requestJson<{ success: boolean }>(
      "POST",
      API_ROUTES.agentAuth.submitCode(provider),
      { code },
    ),

  cancelCodeLogin: (provider: string) =>
    requestJson<{ ok: boolean }>(
      "POST",
      API_ROUTES.agentAuth.cancelCodeLogin(provider),
      {},
    ),

  startDeviceLogin: async (provider: string): Promise<AgentAuthLoginSnapshot> => {
    const login = await requestJson<DeviceLoginStart>(
      "POST",
      API_ROUTES.agentAuth.startDeviceLogin(provider),
      {},
    );
    return {
      active: login.active,
      url: login.verificationUri,
      userCode: login.userCode,
      startedAt: login.startedAt,
      expiresAt: login.expiresAt,
      completed: login.completed,
      error: login.error,
    };
  },

  saveAPIKey: (provider: string, apiKey: string) =>
    requestJson<AgentAuthSnapshot>(
      "POST",
      API_ROUTES.agentAuth.apiKey(provider),
      { apiKey },
    ),

  deleteAPIKey: (provider: string) =>
    requestJson<AgentAuthSnapshot>(
      "DELETE",
      API_ROUTES.agentAuth.apiKey(provider),
    ),

  subscribe: (
    provider: string,
    onStatus: (status: AgentAuthSnapshot) => void,
  ) => subscribeToJsonMessages(WEB_SOCKET_ROUTES.agentAuthStatus(provider), onStatus),
};
