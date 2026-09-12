import { API_ROUTES } from "../config/routes";
import { requestJson } from "./apiRequest";
import type { SaveSMTPSettingsRequest, SMTPSettings } from "../model/email/api/smtpSettings";
import type { EmailSettingsGateway } from "../port/email/outbound/emailSettingsGateway";

export const emailApi: EmailSettingsGateway = {
  get: () => requestJson<SMTPSettings>("GET", API_ROUTES.email.settings),
  save: (request: SaveSMTPSettingsRequest) =>
    requestJson<SMTPSettings>("PUT", API_ROUTES.email.settings, request),
  remove: () => requestJson<void>("DELETE", API_ROUTES.email.settings),
  sendTest: (to: string) => requestJson<{ sent: boolean }>("POST", API_ROUTES.email.test, { to }),
};
