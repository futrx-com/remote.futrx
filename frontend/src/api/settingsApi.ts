import { requestJson } from "./apiRequest";
import {
  type UpdateUserSettingsInput,
  type UserSettings,
} from "../models/settings";
import { API_ROUTES } from "../config/routes";
import {
  DEFAULT_USER_SETTINGS,
  VALID_APPEARANCE_THEMES,
} from "../config/settings";

export const settingsApi = {
  fetch: async () =>
    normalizeUserSettings(
      await requestJson<UserSettings>("GET", API_ROUTES.settings)
    ),
  update: async (body: UpdateUserSettingsInput) =>
    normalizeUserSettings(
      await requestJson<UserSettings>("PATCH", API_ROUTES.settings, body)
    ),
};

function normalizeUserSettings(settings: UserSettings): UserSettings {
  const theme = settings?.appearance?.theme;
  const provider = settings?.chat?.provider;
  const mode = settings?.chat?.mode;
  const reasoningEffort = settings?.chat?.reasoningEffort;
  const serviceTier = settings?.chat?.serviceTier;
  return {
    ...DEFAULT_USER_SETTINGS,
    ...settings,
    appearance: {
      ...DEFAULT_USER_SETTINGS.appearance,
      ...settings?.appearance,
      theme: VALID_APPEARANCE_THEMES.has(theme)
        ? theme
        : DEFAULT_USER_SETTINGS.appearance.theme,
    },
    chat: {
      ...DEFAULT_USER_SETTINGS.chat,
      ...settings?.chat,
      provider: typeof provider === "string" && provider.length > 0
        ? provider
        : DEFAULT_USER_SETTINGS.chat.provider,
      model:
        typeof settings?.chat?.model === "string"
          ? settings.chat.model
          : DEFAULT_USER_SETTINGS.chat.model,
      mode: typeof mode === "string" && mode.length > 0
        ? mode
        : DEFAULT_USER_SETTINGS.chat.mode,
      reasoningEffort: typeof reasoningEffort === "string"
        ? reasoningEffort
        : DEFAULT_USER_SETTINGS.chat.reasoningEffort,
      serviceTier: typeof serviceTier === "string"
        ? serviceTier
        : DEFAULT_USER_SETTINGS.chat.serviceTier,
    },
  };
}
