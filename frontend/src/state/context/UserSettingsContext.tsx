import type { ComponentChildren } from "preact";
import { createContext } from "preact";
import { useCallback, useContext, useEffect, useMemo, useState } from "preact/hooks";
import { useAuthContext } from "./AuthContext";
import {
  type AppearanceTheme,
  type ChatSettings,
  type UserSettings,
} from "../../models/settings";
import { settingsApi } from "../../api/settingsApi";
import { DEFAULT_USER_SETTINGS } from "../../config/settings";
import { appearanceThemeState } from "./appearanceThemeState";

interface UserSettingsContextValue {
  settings: UserSettings;
  loading: boolean;
  saving: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  setTheme: (theme: AppearanceTheme) => Promise<void>;
  setChatSettings: (chat: Partial<ChatSettings>) => Promise<void>;
}

const UserSettingsContext = createContext<UserSettingsContextValue | null>(null);

/**
 * The defaults, with appearance taken from what this browser last applied.
 *
 * The built-in default is "system", and using it before the server answers
 * repaints a light-theme user's app in dark for the length of the round-trip —
 * on every load, and again whenever the gate closes. Worse, applying it also
 * caches "system", so the next cold boot paints its very first frame wrong too.
 */
function settingsFromCachedAppearance(): UserSettings {
  return {
    ...DEFAULT_USER_SETTINGS,
    appearance: { ...DEFAULT_USER_SETTINGS.appearance, theme: appearanceThemeState.remembered() },
  };
}

export function UserSettingsProvider({ children }: { children: ComponentChildren }) {
  ////////////////
  // Local State
  ////////////////
  const { gateOpen } = useAuthContext();
  const [settings, setSettings] = useState<UserSettings>(settingsFromCachedAppearance);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  ////////////////
  // Handlers
  ////////////////
  const refresh = useCallback(async () => {
    if (!gateOpen) {
      setSettings(settingsFromCachedAppearance());
      setLoading(false);
      setError(null);
      return;
    }

    setLoading(true);
    try {
      setSettings(await settingsApi.fetch());
      setError(null);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setLoading(false);
    }
  }, [gateOpen]);

  const setTheme = useCallback(async (theme: AppearanceTheme) => {
    const previous = settings;
    setSettings({ ...settings, appearance: { ...settings.appearance, theme } });
    setSaving(true);
    try {
      setSettings(await settingsApi.update({ appearance: { theme } }));
      setError(null);
    } catch (e) {
      setSettings(previous);
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  }, [settings]);

  const setChatSettings = useCallback(async (chat: Partial<ChatSettings>) => {
    const previous = settings;
    setSettings({ ...settings, chat: { ...settings.chat, ...chat } });
    setSaving(true);
    try {
      setSettings(await settingsApi.update({ chat }));
      setError(null);
    } catch (e) {
      setSettings(previous);
      setError((e as Error).message);
    } finally {
      setSaving(false);
    }
  }, [settings]);

  ////////////////
  // Effects
  ////////////////
  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    appearanceThemeState.apply(settings.appearance.theme);
    return appearanceThemeState.observeSystemChanges(settings.appearance.theme);
  }, [settings.appearance.theme]);

  ////////////////
  // Context Value
  ////////////////
  const value = useMemo<UserSettingsContextValue>(() => ({
    settings,
    loading,
    saving,
    error,
    refresh,
    setTheme,
    setChatSettings,
  }), [settings, loading, saving, error, refresh, setTheme, setChatSettings]);

  return (
    <UserSettingsContext.Provider value={value}>
      {children}
    </UserSettingsContext.Provider>
  );
}

export function useUserSettingsContext(): UserSettingsContextValue {
  const value = useContext(UserSettingsContext);
  if (!value) throw new Error("useUserSettingsContext must be used inside UserSettingsProvider");
  return value;
}
