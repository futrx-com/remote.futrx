const APP_PASSWORD_LENGTH = 16;

interface EmailSettingsFormInput {
  address: string;
  appPassword: string;
}

class EmailSettingsForm {
  prepareSubmission(input: EmailSettingsFormInput) {
    const address = input.address.trim().toLowerCase();
    if (!address) return { valid: false as const, error: "Email address is required." };

    const [local, domain, ...rest] = address.split("@");
    if (!local || !domain || rest.length > 0) {
      return { valid: false as const, error: "Enter a valid email address." };
    }

    const appPassword = input.appPassword.replace(/\s+/g, "");
    if (appPassword.length !== APP_PASSWORD_LENGTH) {
      return {
        valid: false as const,
        error: "A Gmail app password is exactly 16 characters.",
      };
    }

    return { valid: true as const, address, appPassword };
  }
}

export const emailSettingsForm = new EmailSettingsForm();
