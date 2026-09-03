import { MIN_LOCAL_PASSWORD_LENGTH } from "../../../config/auth.ts";
import type { LoginMode } from "../../../models/auth.ts";

interface LocalAuthFormInput {
  mode: LoginMode;
  email: string;
  password: string;
  confirmation: string;
}

class LocalAuthFormState {
  isSetup(mode: LoginMode): boolean {
    return mode === "claim" || mode === "legacy-setup";
  }

  prepareSubmission(input: LocalAuthFormInput) {
    const email = input.email.trim().toLowerCase();
    if (!email) return { valid: false as const, error: "Email is required." };

    if (this.isSetup(input.mode) && input.password !== input.confirmation) {
      return { valid: false as const, error: "Passwords do not match." };
    }

    if (this.isSetup(input.mode) && input.password.length < MIN_LOCAL_PASSWORD_LENGTH) {
      return {
        valid: false as const,
        error: `Use at least ${MIN_LOCAL_PASSWORD_LENGTH} characters.`,
      };
    }

    return { valid: true as const, email };
  }
}

export const localAuthFormState = new LocalAuthFormState();
