import { useEffect, useState } from "preact/hooks";
import { notificationsApi } from "../../../api/notificationsApi";
import type {
  NotificationEventToggles,
  NotificationSettings,
  NotificationTestResult,
} from "../../../models/notifications";

const DEFAULT_EVENTS: NotificationEventToggles = {
  runFinished: true,
  runFailed: true,
  needsAttention: true,
  scheduledRun: true,
};

export function useNotificationsSettingsController() {
  const [settings, setSettings] = useState<NotificationSettings | null>(null);
  const [notificationsEnabled, setNotificationsEnabled] = useState(false);
  const [botToken, setBotToken] = useState("");
  const [chatId, setChatId] = useState("");
  const [webhookUrl, setWebhookUrl] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [events, setEvents] = useState<NotificationEventToggles>(DEFAULT_EVENTS);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const [testResults, setTestResults] = useState<NotificationTestResult[] | null>(null);

  function adopt(value: NotificationSettings) {
    setSettings(value);
    setNotificationsEnabled(value.enabled);
    setChatId(value.telegram.chatId ?? "");
    setWebhookUrl(value.webhook.url ?? "");
    setEvents(value.events ?? DEFAULT_EVENTS);
    // Secrets are never returned, so the inputs stay empty and an empty
    // submission means "keep what the server already has".
    setBotToken("");
    setWebhookSecret("");
  }

  useEffect(() => {
    let cancelled = false;
    notificationsApi
      .get()
      .then((value) => {
        if (cancelled) return;
        adopt(value);
      })
      .catch((cause) => !cancelled && setError((cause as Error).message))
      .finally(() => !cancelled && setLoading(false));
    return () => {
      cancelled = true;
    };
  }, []);

  function enableNotifications(value: boolean) {
    setNotificationsEnabled(value);
    setSaved(false);
  }

  function toggleEvent(key: keyof NotificationEventToggles, value: boolean) {
    setEvents((current) => ({ ...current, [key]: value }));
    setSaved(false);
  }

  async function save(event: Event) {
    event.preventDefault();
    setSaving(true);
    setError(null);
    setSaved(false);
    setTestResults(null);
    try {
      const value = await notificationsApi.save({
        enabled: notificationsEnabled,
        telegram: { botToken: botToken.trim(), chatId: chatId.trim() },
        webhook: { url: webhookUrl.trim(), secret: webhookSecret.trim() },
        events,
      });
      adopt(value);
      setSaved(true);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function clearTelegramToken() {
    await clearSecret({ clearBotToken: true });
  }

  async function clearWebhookSecret() {
    await clearSecret({ clearSecret: true });
  }

  async function clearSecret(flags: { clearBotToken?: boolean; clearSecret?: boolean }) {
    setSaving(true);
    setError(null);
    setSaved(false);
    try {
      const value = await notificationsApi.save({
        enabled: notificationsEnabled,
        telegram: {
          botToken: "",
          clearBotToken: flags.clearBotToken === true,
          chatId: chatId.trim(),
        },
        webhook: {
          url: webhookUrl.trim(),
          secret: "",
          clearSecret: flags.clearSecret === true,
        },
        events,
      });
      adopt(value);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function sendTest() {
    setTesting(true);
    setError(null);
    setTestResults(null);
    try {
      const response = await notificationsApi.test();
      setTestResults(response.results);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setTesting(false);
    }
  }

  return {
    botToken,
    chatId,
    clearTelegramToken,
    clearWebhookSecret,
    error,
    events,
    loading,
    notificationsEnabled,
    save,
    saved,
    saving,
    sendTest,
    setBotToken,
    setChatId,
    setNotificationsEnabled: enableNotifications,
    setWebhookSecret,
    setWebhookUrl,
    settings,
    testing,
    testResults,
    toggleEvent,
    webhookSecret,
    webhookUrl,
  };
}
