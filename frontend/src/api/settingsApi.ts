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
  const chat = normalizeChatSettings(settings?.chat, DEFAULT_USER_SETTINGS.chat);
  // Older servers and stored settings only expose `chat`. Treat it as the
  // project preference too until the user chooses a project-specific agent.
  const projectChat = normalizeChatSettings(settings?.projectChat, chat);
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
    chat,
    projectChat,
  };
}

function normalizeChatSettings(
  settings: UserSettings["chat"] | undefined,
  defaults: UserSettings["chat"]
): UserSettings["chat"] {
  const provider = settings?.provider;
  const mode = settings?.mode;
  const reasoningEffort = settings?.reasoningEffort;
  const serviceTier = settings?.serviceTier;
  return {
    ...defaults,
    ...settings,
    provider: typeof provider === "string" && provider.length > 0
      ? provider
      : defaults.provider,
    model: typeof settings?.model === "string" ? settings.model : defaults.model,
    mode: typeof mode === "string" && mode.length > 0 ? mode : defaults.mode,
    reasoningEffort: typeof reasoningEffort === "string"
      ? reasoningEffort
      : defaults.reasoningEffort,
    serviceTier: typeof serviceTier === "string" ? serviceTier : defaults.serviceTier,
  };
}
