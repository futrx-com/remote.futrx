import { sendHttpRequest } from "../transport/http";
import { requestJson } from "./apiRequest";
import type {
  AuthSession,
  GoogleOAuthSettings,
  InitialPasswordResult,
  LocalLoginResult,
} from "../models/auth";
import { ApiError } from "./apiError";
import { API_ROUTES } from "../config/routes";
import { UNAUTHENTICATED_SESSION } from "../config/auth";

export async function fetchAuthSession(): Promise<AuthSession> {
  try {
    const response = await sendHttpRequest("GET", API_ROUTES.authSession);
    if (!response.ok) return UNAUTHENTICATED_SESSION;
    const data = await response.json();
    return {
      authenticated: !!data.authenticated,
      claimed: !!data.claimed,
      localAdminConfigured: !!data.localAdminConfigured,
      googleOAuthEnabled: !!data.googleOAuthEnabled,
      googleClientId: data.googleClientId ?? "",
      adminEmail: data.adminEmail ?? "",
      email: data.email ?? "",
      isAdmin: !!data.isAdmin,
      isRegistered: !!data.isRegistered,
    };
  } catch {
    return UNAUTHENTICATED_SESSION;
  }
}

export const localAuthApi = {
  // setupToken is sent in the body, never the query string, so it stays out
  // of reverse-proxy access logs.
  claim: (email: string, password: string, setupToken: string) =>
    requestPreSessionJson<AuthSession>("/auth/local/claim", { email, password, setupToken }),
  login: (email: string, password: string) =>
    requestLocalAuthOrChallenge("/auth/local/login", email, password),
};

// twoFactorApi completes (or cancels) the pending-login challenge issued by
// localAuthApi.login (or the Google callback redirect) when the account has
// 2FA enabled. Pre-session, like localAuthApi, so it talks to sendHttpRequest
// directly rather than requestJson - a wrong code intentionally returns 401
// here and must not trigger requestJson's "session expired, reload" behavior.
export const twoFactorApi = {
  verify: (code: string) =>
    requestPreSessionJson<AuthSession>(API_ROUTES.auth2fa.verify, { code }),
  cancel: async (): Promise<void> => {
    await sendHttpRequest("POST", API_ROUTES.auth2fa.cancel);
  },
};

// initialPasswordApi finishes the set-password step an invited account is
// held at after signing in with a temporary password. Pre-session like the
// 2FA calls: the server answers a stale step with 400, never 401, so the
// message and status reach the caller instead of reloading the page.
export const initialPasswordApi = {
  submit: async (newPassword: string): Promise<InitialPasswordResult> => {
    const data = await requestPreSessionJson<
      { twoFactorRequired: true } | (AuthSession & { twoFactorRequired?: false })
    >(API_ROUTES.localPassword.initial, { newPassword });
    if (data.twoFactorRequired) return { outcome: "two-factor" };
    return { outcome: "session", session: data };
  },
  cancel: async (): Promise<void> => {
    await sendHttpRequest("POST", API_ROUTES.localPassword.cancel);
  },
};

export const googleOAuthApi = {
  get: () =>
    requestJson<GoogleOAuthSettings>("GET", API_ROUTES.googleOAuth),
  save: (clientId: string, clientSecret: string) =>
    requestJson<GoogleOAuthSettings>("PUT", API_ROUTES.googleOAuth, {
      clientId,
      clientSecret,
    }),
};

async function requestPreSessionJson<T>(url: string, body: unknown): Promise<T> {
  const response = await sendHttpRequest("POST", url, body);
  if (!response.ok) {
    let message = `${response.status}`;
    try {
      message = (await response.json()).error || message;
    } catch {}
    throw new ApiError(message, response.status);
  }
  return response.json() as Promise<T>;
}

// A challenge reply carries only its flag; a completed one is a full session.
type LocalLoginResponse = Partial<AuthSession> & {
  twoFactorRequired?: boolean;
  passwordChangeRequired?: boolean;
};

// Local login's response shape branches: an account with 2FA enabled gets
// {twoFactorRequired: true} and a pending cookie, and one still on a
// temporary password gets {passwordChangeRequired: true} and a change cookie,
// instead of a full AuthSession.
async function requestLocalAuthOrChallenge(
  url: string,
  email: string,
  password: string
): Promise<LocalLoginResult> {
  const data = await requestPreSessionJson<LocalLoginResponse>(url, {
    email,
    password,
  });
  if (data.twoFactorRequired) return { outcome: "two-factor" };
  if (data.passwordChangeRequired) return { outcome: "password-change" };
  return { outcome: "session", session: data as AuthSession };
}
