import { MAX_LOCAL_PASSWORD_BYTES, MIN_LOCAL_PASSWORD_LENGTH } from "../../../config/auth.ts";
import type { PasswordChangeInput } from "../../../models/security.ts";

interface PasswordChangeFormInput extends PasswordChangeInput {
  confirmation: string;
  twoFactorEnabled: boolean;
}

type Submission = { valid: false; error: string } | { valid: true; input: PasswordChangeInput };

export const passwordChangeFormState = {
  prepareSubmission(input: PasswordChangeFormInput): Submission {
    if (!input.currentPassword) {
      return { valid: false, error: "Current password is required." };
    }
    if ([...input.newPassword].length < MIN_LOCAL_PASSWORD_LENGTH) {
      return { valid: false, error: `Use at least ${MIN_LOCAL_PASSWORD_LENGTH} characters.` };
    }
    if (new TextEncoder().encode(input.newPassword).byteLength > MAX_LOCAL_PASSWORD_BYTES) {
      return { valid: false, error: `Use at most ${MAX_LOCAL_PASSWORD_BYTES} bytes.` };
    }
    if (input.newPassword !== input.confirmation) {
      return { valid: false, error: "Passwords do not match." };
    }
    if (input.newPassword === input.currentPassword) {
      return { valid: false, error: "New password must differ from your current password." };
    }
    const code = input.code?.trim() ?? "";
    if (input.twoFactorEnabled && !code) {
      return { valid: false, error: "Enter an authenticator code or recovery code." };
    }
    return {
      valid: true,
      input: {
        currentPassword: input.currentPassword,
        newPassword: input.newPassword,
        ...(input.twoFactorEnabled ? { code } : {}),
      },
    };
  },
};
