import type { AppearanceTheme, UserSettings } from "../models/settings";

export const DEFAULT_USER_SETTINGS: UserSettings = {
  appearance: { theme: "system" },
  chat: {
    provider: "codex",
    model: "",
    mode: "default",
    reasoningEffort: "",
    serviceTier: "",
    approvalPolicy: "on-request",
    sandboxPolicy: "workspaceWrite",
  },
  projectChat: {
    provider: "codex",
    model: "",
    mode: "default",
    reasoningEffort: "",
    serviceTier: "",
    approvalPolicy: "on-request",
    sandboxPolicy: "workspaceWrite",
  },
};

export const VALID_APPEARANCE_THEMES = new Set<AppearanceTheme>([
  "system",
  "dark",
  "light",
]);
export const SYSTEM_LIGHT_MEDIA_QUERY = "(prefers-color-scheme: light)";
