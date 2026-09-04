export type SignInMethod =
  | "password"
  | "google"
  | "password+totp"
  | "google+totp"
  | "password+recovery-code"
  | "google+recovery-code";

export interface SessionHistoryEntry {
  sid: string;
  method: SignInMethod;
  ip: string;
  userAgent: string;
  issuedAt: number;
}

export interface SecurityAlertSummary {
  method: SignInMethod;
  ip: string;
  userAgent: string;
  occurredAt: number;
  acknowledged: boolean;
}

// SecuritySettings mirrors the backend's SecuritySummary: 2FA status plus
// the three independent SecurityPreferences toggles (single active session,
// sign-in history, recovery-code alert - the last only meaningful/settable
// while 2FA is enabled).
export interface SecuritySettings {
  twoFactorEnabled: boolean;
  recoveryCodesRemaining: number;
  singleSessionEnabled: boolean;
  historyEnabled: boolean;
  recoveryCodeAlertEnabled: boolean;
  sessions: SessionHistoryEntry[];
  securityAlert?: SecurityAlertSummary;
}

export interface TwoFactorEnrollment {
  enrollmentToken: string;
  secret: string;
  otpauthUrl: string;
}

export type SecurityPreferencesInput = Partial<
  Pick<
    SecuritySettings,
    "singleSessionEnabled" | "historyEnabled" | "recoveryCodeAlertEnabled"
  >
>;
