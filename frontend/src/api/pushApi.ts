import { requestJson } from "./apiRequest.ts";
import { API_ROUTES } from "../config/routes.ts";
import { sendHttpRequest } from "../transport/http.ts";
import type {
  PushConfig,
  PushPresencePayload,
  PushSubscriptionPayload,
  PushSubscriptionStatus,
} from "../models/push";

export const pushApi = {
  config: () => requestJson<PushConfig>("GET", API_ROUTES.push.config),
  subscribe: (subscription: PushSubscriptionPayload) =>
    requestJson<void>("POST", API_ROUTES.push.subscriptions, subscription),
  unsubscribe: (endpoint: string) =>
    requestJson<void>("DELETE", API_ROUTES.push.subscriptions, { endpoint }),
  subscriptionStatus: (endpoint: string) =>
    requestJson<PushSubscriptionStatus>("POST", API_ROUTES.push.subscriptionStatus, {
      endpoint,
    }),
  test: () => requestJson<void>("POST", API_ROUTES.push.test),
  presence: (payload: PushPresencePayload, keepalive = false) =>
    sendPresence(payload, keepalive),
};

/**
 * Reports what this client has on screen.
 *
 * Deliberately not routed through requestJson: this is background traffic, and
 * requestJson reloads the page on a 401 — which would yank the app out from
 * under someone mid-sentence just because a heartbeat raced a session refresh.
 * `keepalive` lets a withdrawal outlive the page that is unloading.
 */
async function sendPresence(payload: PushPresencePayload, keepalive: boolean): Promise<void> {
  const response = await sendHttpRequest(
    "POST",
    API_ROUTES.push.presence,
    payload,
    keepalive ? { keepalive: true } : undefined
  );
  if (!response.ok) throw new Error(`presence: ${response.status}`);
}
