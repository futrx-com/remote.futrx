import { useCallback, useEffect, useState } from "preact/hooks";
import { securityApi } from "../../../api/securityApi";
import type {
  SecurityPreferencesInput,
  SecuritySettings,
  TwoFactorEnrollment,
} from "../../../models/security";

export interface SecuritySettingsController {
  settings: SecuritySettings | null;
  loading: boolean;
  error: string | null;
  refresh: () => Promise<void>;
  beginEnrollment: () => Promise<TwoFactorEnrollment>;
  confirmEnrollment: (enrollmentToken: string, code: string) => Promise<string[]>;
  disable: (code: string) => Promise<void>;
  regenerateRecoveryCodes: (code: string) => Promise<string[]>;
  setPreferences: (input: SecurityPreferencesInput) => Promise<void>;
  ackAlert: () => Promise<void>;
}

export function useSecuritySettings(enabled: boolean): SecuritySettingsController {
  const [settings, setSettings] = useState<SecuritySettings | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setSettings(await securityApi.fetchSettings());
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (enabled) void refresh();
  }, [enabled, refresh]);

  const beginEnrollment = useCallback(() => securityApi.beginTwoFactorEnrollment(), []);

  const confirmEnrollment = useCallback(
    async (enrollmentToken: string, code: string) => {
      const recoveryCodes = await securityApi.confirmTwoFactorEnrollment(enrollmentToken, code);
      await refresh();
      return recoveryCodes;
    },
    [refresh]
  );

  const disable = useCallback(
    async (code: string) => {
      await securityApi.disableTwoFactor(code);
      await refresh();
    },
    [refresh]
  );

  const regenerateRecoveryCodes = useCallback(async (code: string) => {
    return securityApi.regenerateRecoveryCodes(code);
  }, []);

  const setPreferences = useCallback(async (input: SecurityPreferencesInput) => {
    setSettings(await securityApi.updatePreferences(input));
  }, []);

  const ackAlert = useCallback(async () => {
    await securityApi.acknowledgeAlert();
    setSettings((current) => (current ? { ...current, securityAlert: undefined } : current));
  }, []);

  return {
    settings,
    loading,
    error,
    refresh,
    beginEnrollment,
    confirmEnrollment,
    disable,
    regenerateRecoveryCodes,
    setPreferences,
    ackAlert,
  };
}
