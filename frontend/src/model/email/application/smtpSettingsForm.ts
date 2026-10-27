import {
  EMAIL_PROVIDER_PRESETS,
  GMAIL_APP_PASSWORD_LENGTH,
  type AuthenticationMode,
  type TLSMode,
} from "../../../config/constants/email-providers.ts";
import type {
  PublicSMTPConfiguration,
  SaveSMTPSettingsRequest,
} from "../api/smtpSettings.ts";

export type EmailProviderChoice = "gmail" | "custom";

export interface SMTPFormInput {
  providerId: EmailProviderChoice;
  host: string;
  port: string;
  tlsMode: TLSMode;
  authentication: AuthenticationMode;
  username: string;
  /** Raw field value. Empty while editing a configuration that already has a
   * stored secret means "retain it"; empty on a first save is an error. */
  password: string;
  fromAddress: string;
  passwordConfigured: boolean;
}

export type PrepareSubmissionResult =
  | { valid: true; request: SaveSMTPSettingsRequest }
  | { valid: false; error: string };

class SMTPSettingsForm {
  /** A fresh Custom SMTP form: STARTTLS + PLAIN, no Gmail values copied. */
  blankCustomInput(): SMTPFormInput {
    return {
      providerId: "custom",
      host: "",
      port: "",
      tlsMode: "starttls",
      authentication: "plain",
      username: "",
      password: "",
      fromAddress: "",
      passwordConfigured: false,
    };
  }

  /** A fresh Gmail form, filled from the one canonical preset. */
  blankGmailInput(): SMTPFormInput {
    const preset = EMAIL_PROVIDER_PRESETS.gmail;
    return {
      providerId: "gmail",
      host: preset.host,
      port: String(preset.port),
      tlsMode: preset.tlsMode,
      authentication: preset.authentication,
      username: "",
      password: "",
      fromAddress: "",
      passwordConfigured: false,
    };
  }

  /** Classifies a stored configuration as Gmail only when every public field
   * exactly matches the Gmail preset; any mismatch is Custom SMTP. The
   * backend never returns a provider id. */
  classify(cfg: PublicSMTPConfiguration): EmailProviderChoice {
    const preset = EMAIL_PROVIDER_PRESETS.gmail;
    const isGmail =
      cfg.host === preset.host &&
      cfg.port === preset.port &&
      cfg.tlsMode === preset.tlsMode &&
      cfg.authentication === preset.authentication;
    return isGmail ? "gmail" : "custom";
  }

  toInput(cfg: PublicSMTPConfiguration): SMTPFormInput {
    return {
      providerId: this.classify(cfg),
      host: cfg.host,
      port: String(cfg.port),
      tlsMode: cfg.tlsMode,
      authentication: cfg.authentication,
      username: cfg.username,
      password: "",
      fromAddress: cfg.fromAddress,
      passwordConfigured: cfg.passwordConfigured,
    };
  }

  prepareSubmission(input: SMTPFormInput): PrepareSubmissionResult {
    const host = input.host.trim();
    if (!host) return { valid: false, error: "Host is required." };

    const port = Number(input.port);
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      return { valid: false, error: "Port must be between 1 and 65535." };
    }

    const fromAddress = input.fromAddress.trim().toLowerCase();
    const [local, domain, ...rest] = fromAddress.split("@");
    if (!local || !domain || rest.length > 0) {
      return { valid: false, error: "Enter a valid sender address." };
    }

    if (input.tlsMode === "none" && input.authentication !== "none") {
      return {
        valid: false,
        error: "A plaintext connection cannot be used with a password.",
      };
    }

    if (input.authentication === "none") {
      return {
        valid: true,
        request: {
          host,
          port,
          tlsMode: input.tlsMode,
          authentication: "none",
          username: "",
          password: undefined,
          fromAddress,
        },
      };
    }

    const username = input.username.trim();
    if (!username) return { valid: false, error: "Username is required." };

    let password: string | undefined;
    if (input.providerId === "gmail") {
      const stripped = input.password.replace(/\s+/g, "");
      if (stripped === "") {
        if (!input.passwordConfigured) {
          return { valid: false, error: "A Gmail app password is required." };
        }
        password = undefined;
      } else if (stripped.length !== GMAIL_APP_PASSWORD_LENGTH) {
        return {
          valid: false,
          error: "A Gmail app password is exactly 16 characters.",
        };
      } else {
        password = stripped;
      }
    } else if (input.password === "") {
      if (!input.passwordConfigured) {
        return { valid: false, error: "A password is required." };
      }
      password = undefined;
    } else {
      password = input.password;
    }

    return {
      valid: true,
      request: {
        host,
        port,
        tlsMode: input.tlsMode,
        authentication: input.authentication,
        username,
        password,
        fromAddress,
      },
    };
  }
}

export const smtpSettingsForm = new SMTPSettingsForm();
