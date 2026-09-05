import { API_ROUTES } from "../config/routes";
import { requestJson } from "./apiRequest";
import type { EmailSettings } from "../models/email";

export const emailApi = {
  get: () => requestJson<EmailSettings>("GET", API_ROUTES.email.settings),
  save: (address: string, appPassword: string) =>
    requestJson<EmailSettings>("PUT", API_ROUTES.email.settings, { address, appPassword }),
  remove: () => requestJson<undefined>("DELETE", API_ROUTES.email.settings),
  sendTest: (to: string) => requestJson<{ sent: boolean }>("POST", API_ROUTES.email.test, { to }),
};
