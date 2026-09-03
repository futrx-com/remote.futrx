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

export type AgentAuthMode = "managed-code" | "managed-device" | "external" | "none";

/** How one provider's login stands, as the settings row draws it. */
export type AgentAuthStatusKind = "no-auth" | "authenticated" | "external" | "unconfigured";

export interface AgentAuthLoginSnapshot {
  active: boolean;
  url?: string;
  awaitingCode?: boolean;
  userCode?: string;
  startedAt?: number;
  expiresAt?: number;
  completed?: boolean;
  error?: string;
}

export interface AgentAuthSnapshot {
  authenticated: boolean;
  warning?: string;
  login: AgentAuthLoginSnapshot;
}

export interface AgentAuthProvider {
  provider: string;
  label: string;
  default?: boolean;
  executionScopes: Array<"host" | "project">;
  authentication: {
    mode: AgentAuthMode;
    instructions?: string;
    satisfiesAccessGate: boolean;
  };
  status: AgentAuthSnapshot;
}

export interface AgentAuthCatalog {
  providers: AgentAuthProvider[];
}

/** Which question the local login screen is asking. */
export type LoginMode = "claim" | "login" | "legacy-setup";
