// Wire-format boundary types for the admin SMTP settings API. This mirrors
// the backend's model/email/api package - the field names and enum wire
// values are decided there; this file only reflects that process boundary.
import type { AuthenticationMode, TLSMode } from "../../../config/constants/email-providers.ts";

export interface PublicSMTPConfiguration {
  host: string;
  port: number;
  tlsMode: TLSMode;
  authentication: AuthenticationMode;
  username: string;
  fromAddress: string;
  passwordConfigured: boolean;
}

export interface SMTPSettings {
  configured: boolean;
  configuration?: PublicSMTPConfiguration;
}

export interface SaveSMTPSettingsRequest {
  host: string;
  port: number;
  tlsMode: TLSMode;
  authentication: AuthenticationMode;
  username: string;
  password?: string;
  fromAddress: string;
}
