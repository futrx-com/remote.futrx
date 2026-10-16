import type { InviteMethod } from "../../../models/user.ts";

export interface InviteChannels {
  googleConfigured: boolean;
  smtpConfigured: boolean;
}

/** What the admin chose explicitly; `null` means "leave it to the default". */
export interface InviteChoices {
  method: InviteMethod | null;
  sendEmail: boolean | null;
}

export interface InviteFormView {
  methods: InviteMethod[];
  method: InviteMethod;
  /** The email checkbox exists only for the password method with SMTP set up. */
  showEmailOption: boolean;
  sendEmail: boolean;
  /** Shown instead of the checkbox when there is no SMTP to send with. */
  emailHint: string | null;
}

export const SMTP_NOT_CONFIGURED_HINT = "SMTP is not configured; you'll share the credentials yourself";

// Which sign-in methods and delivery options the invite form offers, derived
// from what the server has set up. Defaults follow the setup, so a choice the
// admin has not made yet moves with it (SMTP finishing its load, say) while
// one they have made sticks.
export const inviteFormState = {
  view(channels: InviteChannels, choices: InviteChoices): InviteFormView {
    const methods: InviteMethod[] = channels.googleConfigured ? ["google", "password"] : ["password"];
    const method = choices.method && methods.includes(choices.method) ? choices.method : "password";
    const emailApplies = method === "password";
    return {
      methods,
      method,
      showEmailOption: emailApplies && channels.smtpConfigured,
      sendEmail: emailApplies && channels.smtpConfigured && (choices.sendEmail ?? true),
      emailHint: emailApplies && !channels.smtpConfigured ? SMTP_NOT_CONFIGURED_HINT : null,
    };
  },
};
