import type { AuthSession } from "../models/auth";

export const UNAUTHENTICATED_SESSION: AuthSession = {
  authenticated: false,
  claimed: false,
  localAdminConfigured: false,
  googleOAuthEnabled: false,
  googleClientId: "",
  adminEmail: "",
  email: "",
  isAdmin: false,
  isRegistered: false,
};

/** How often the setup screen re-checks whether an admin has been configured. */
export const ADMIN_SETUP_POLL_INTERVAL_MS = 3_000;

/** Minimum Unicode characters for a local password. */
export const MIN_LOCAL_PASSWORD_LENGTH = 12;
/** Maximum UTF-8 bytes for a local password. */
export const MAX_LOCAL_PASSWORD_BYTES = 1_024;
/** Longest `return_to` the login flow will even look at. */
export const MAX_RETURN_URL_LENGTH = 2_048;
