import { requestJson } from "../../apiRequest";
import { subscribeToJsonMessages } from "../../../transport/jsonMessageSubscription";
import type { CustomAuthStatus, CustomProviderConfig } from "../../../models/auth";
import { API_ROUTES, WEB_SOCKET_ROUTES } from "../../../config/routes";

export const customAuthApi = {
  fetchStatus: () =>
    requestJson<CustomAuthStatus>("GET", API_ROUTES.customAuth.status),
  save: (config: CustomProviderConfig) =>
    requestJson<{ success: boolean }>("POST", API_ROUTES.customAuth.save, config),
  subscribe: (onStatus: (status: CustomAuthStatus) => void) =>
    subscribeToJsonMessages(WEB_SOCKET_ROUTES.customAuthStatus, onStatus),
};
