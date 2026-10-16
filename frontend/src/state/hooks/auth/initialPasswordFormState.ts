import { MAX_LOCAL_PASSWORD_BYTES, MIN_LOCAL_PASSWORD_LENGTH } from "../../../config/auth.ts";

type Submission = { valid: false; error: string } | { valid: true; newPassword: string };

/** What a failed set-password call means for the screen. */
export type InitialPasswordFailure =
  | { kind: "expired"; message: string }
  | { kind: "rejected"; message: string };

// The server's reply for a stale or forged change cookie. It is a 400 like the
// password rules are (a 401 would sign the SPA out), so the text is how the two
// are told apart.
const EXPIRED_STEP_MESSAGE = "this sign-in step expired, sign in again";
const GENERIC_FAILURE = "Could not set the new password. Try again.";

export const initialPasswordFormState = {
  // Mirrors what the server checks first so the common slips fail without a
  // round trip. "Differs from the temporary password" is the server's call:
  // the browser never holds the temporary password here.
  prepareSubmission(input: { newPassword: string; confirmation: string }): Submission {
    if ([...input.newPassword].length < MIN_LOCAL_PASSWORD_LENGTH) {
      return { valid: false, error: `Use at least ${MIN_LOCAL_PASSWORD_LENGTH} characters.` };
    }
    if (new TextEncoder().encode(input.newPassword).byteLength > MAX_LOCAL_PASSWORD_BYTES) {
      return { valid: false, error: `Use at most ${MAX_LOCAL_PASSWORD_BYTES} bytes.` };
    }
    if (input.newPassword !== input.confirmation) {
      return { valid: false, error: "Passwords do not match." };
    }
    return { valid: true, newPassword: input.newPassword };
  },

  classifyFailure(status: number | undefined, message: string): InitialPasswordFailure {
    const text = message.trim();
    if (status === 400 && text.toLowerCase().startsWith(EXPIRED_STEP_MESSAGE)) {
      return { kind: "expired", message: "That sign-in step expired. Sign in again." };
    }
    return { kind: "rejected", message: text && !/^\d+$/.test(text) ? text : GENERIC_FAILURE };
  },
};
