export interface NotificationEventToggles {
  runFinished: boolean;
  runFailed: boolean;
  needsAttention: boolean;
  scheduledRun: boolean;
}

/** Telegram credentials as the server reports them: the token is never echoed. */
export interface NotificationTelegramSettings {
  configured: boolean;
  botTokenMasked?: string;
  chatId?: string;
}

/** Webhook target as the server reports it: the shared secret is never echoed. */
export interface NotificationWebhookSettings {
  configured: boolean;
  url?: string;
  secretMasked?: string;
}

export interface NotificationSettings {
  enabled: boolean;
  telegram: NotificationTelegramSettings;
  webhook: NotificationWebhookSettings;
  events: NotificationEventToggles;
  updatedAt?: number;
}

/**
 * Write payload. Blank secrets keep whatever the server already stores, which
 * is why the UI can show a mask instead of the real value; the clear flags are
 * the explicit way to remove one.
 */
export interface UpdateNotificationSettingsInput {
  enabled: boolean;
  telegram: {
    botToken: string;
    clearBotToken?: boolean;
    chatId: string;
  };
  webhook: {
    url: string;
    secret: string;
    clearSecret?: boolean;
  };
  events: NotificationEventToggles;
}

export interface NotificationTestResult {
  sink: string;
  configured: boolean;
  delivered: boolean;
  error?: string;
}
