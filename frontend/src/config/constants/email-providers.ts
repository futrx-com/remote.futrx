// Canonical public preset catalog for the email settings UI. Values only, no
// I/O or state mutation. This is the one owner of Gmail's public SMTP
// defaults anywhere in this codebase — the backend contains none of them.

export type TLSMode = "starttls" | "implicit" | "none";
export type AuthenticationMode = "plain" | "login" | "none";

export interface EmailProviderPreset {
  id: string;
  label: string;
  recommended: boolean;
  host: string;
  port: number;
  tlsMode: TLSMode;
  authentication: AuthenticationMode;
  passwordKind: "gmail-app-password" | "generic";
}

export const EMAIL_PROVIDER_PRESETS = {
  gmail: {
    id: "gmail",
    label: "Gmail",
    recommended: true,
    host: "smtp.gmail.com",
    port: 587,
    tlsMode: "starttls",
    authentication: "plain",
    passwordKind: "gmail-app-password",
  },
} as const satisfies Record<string, EmailProviderPreset>;

export const GMAIL_APP_PASSWORD_LENGTH = 16;

export const TLS_MODE_OPTIONS: { value: TLSMode; label: string }[] = [
  { value: "starttls", label: "STARTTLS" },
  { value: "implicit", label: "Implicit TLS" },
  { value: "none", label: "None (trusted relay only)" },
];

export const AUTHENTICATION_MODE_OPTIONS: { value: AuthenticationMode; label: string }[] = [
  { value: "plain", label: "PLAIN" },
  { value: "login", label: "LOGIN" },
  { value: "none", label: "None" },
];
