import { requestJson } from "./apiRequest";
import type {
  SecurityPreferencesInput,
  SecuritySettings,
  TwoFactorEnrollment,
} from "../models/security";
import { API_ROUTES } from "../config/routes";

export const securityApi = {
  fetchSettings: () => requestJson<SecuritySettings>("GET", API_ROUTES.security.summary),
  beginTwoFactorEnrollment: () =>
    requestJson<TwoFactorEnrollment>("POST", API_ROUTES.security.enroll),
  confirmTwoFactorEnrollment: (enrollmentToken: string, code: string) =>
    requestRecoveryCodes(API_ROUTES.security.confirm, {
      enrollmentToken,
      code,
    }),
  disableTwoFactor: (code: string) =>
    requestJson<void>("POST", API_ROUTES.security.disable, { code }),
  regenerateRecoveryCodes: (code: string) =>
    requestRecoveryCodes(API_ROUTES.security.regenerateRecoveryCodes, { code }),
  updatePreferences: (input: SecurityPreferencesInput) =>
    requestJson<SecuritySettings>("POST", API_ROUTES.security.preferences, input),
  acknowledgeAlert: () => requestJson<void>("POST", API_ROUTES.security.ackAlert),
};

async function requestRecoveryCodes(url: string, body: unknown): Promise<string[]> {
  const response = await requestJson<{ recoveryCodes: string[] }>("POST", url, body);
  return response.recoveryCodes;
}
