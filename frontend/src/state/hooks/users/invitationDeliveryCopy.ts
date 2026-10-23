import type { PasswordInviteResult } from "../../../models/user.ts";

/** How strongly the card draws attention to the delivery outcome. */
export type InvitationDeliveryTone = "success" | "neutral" | "warning" | "error";

export interface InvitationDeliveryCopy {
  tone: InvitationDeliveryTone;
  message: string;
  /** The credentials are on screen at once, rather than behind a question. */
  credentialsVisible: boolean;
  /** The question asked before showing credentials that were already emailed. */
  revealQuestion?: string;
}

// What the admin is told once a password invite or reset has been delivered
// (or not). Only the "sent" outcome hides the credentials: the user already
// has them, so showing them again is a choice.
export const invitationDeliveryCopy = {
  describe(result: PasswordInviteResult): InvitationDeliveryCopy {
    switch (result.delivery) {
      case "sent":
        return {
          tone: "success",
          message: `Invitation emailed to ${result.user.email}`,
          credentialsVisible: false,
          revealQuestion: "View or download the credentials too?",
        };
      case "not_requested":
        return {
          tone: "neutral",
          message: `Credentials for ${result.user.email}. Share them yourself.`,
          credentialsVisible: true,
        };
      case "not_configured":
        return {
          tone: "warning",
          message: "Not emailed: SMTP is not configured",
          credentialsVisible: true,
        };
      case "failed": {
        const reason = (result.deliveryError ?? "").trim().replace(/\.+$/, "");
        return {
          tone: "error",
          message: reason
            ? `The email could not be sent: ${reason}. The account was created.`
            : "The email could not be sent. The account was created.",
          credentialsVisible: true,
        };
      }
    }
  },
};
