export interface AuthSession {
  authenticated: boolean;
  claimed: boolean;
  localAdminConfigured: boolean;
  googleOAuthEnabled: boolean;
  googleClientId: string;
  adminEmail: string;
  email: string;
  isAdmin: boolean;
  isRegistered: boolean;
}

export interface GoogleOAuthSettings {
  configured: boolean;
  clientId: string;
  redirectUrl: string;
}

export interface ClaudeAuthStatus {
  authenticated: boolean;
  login?: ClaudeLoginState;
}

// Streamed handshake state, mirroring CodexDeviceLogin. Claude's CLI uses the
// authorization-code grant instead of a device grant: it emits an OAuth URL
// and expects a code pasted back, so there's an `authUrl` + `awaitingCode`
// rather than a `userCode` for the user to read off.
export interface ClaudeLoginState {
  active: boolean;
  authUrl?: string;
  awaitingCode?: boolean;
  startedAt?: number;
  completed?: boolean;
  error?: string;
}

export interface ClaudeLoginStart {
  url: string;
  resumed?: boolean;
}

export interface CodexAuthStatus {
  authenticated: boolean;
  authMode?: string;
  usesApiKey?: boolean;
  deviceLogin?: CodexDeviceLogin;
}

export interface CodexDeviceLogin {
  active: boolean;
  verificationUri?: string;
  userCode?: string;
  startedAt?: number;
  expiresAt?: number;
  completed?: boolean;
  error?: string;
}

export interface KimiAuthStatus {
  authenticated: boolean;
  deviceLogin?: KimiDeviceLogin;
}

export interface KimiDeviceLogin {
  active: boolean;
  verificationUri?: string;
  userCode?: string;
  startedAt?: number;
  expiresAt?: number;
  completed?: boolean;
  error?: string;
}

export interface CustomProviderConfig {
  name: string;
  apiKey: string;
  baseUrl: string;
}

// CustomAuthStatus mirrors the backend APIKeyStatus. The API key is never
// included; only the saved display name and base URL are surfaced.
export interface CustomAuthStatus {
  authenticated: boolean;
  config?: { name: string; baseUrl: string };
}

export type ClaudeLoginPhase =
  | "idle"
  | "starting"
  | "awaiting-code"
  | "submitting"
  | "done"
  | "error";
