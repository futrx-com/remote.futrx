import { useCallback, useEffect, useState } from "preact/hooks";
import { securityApi } from "../../../api/securityApi";
import type {
  SecurityPreferencesInput,
  SecuritySettings,
  TwoFactorEnrollment,
} from "../../../models/security";

export interface TwoFactorSettingsActions {
  beginTwoFactorEnrollment: () => Promise<TwoFactorEnrollment>;
  confirmTwoFactorEnrollment: (enrollmentToken: string, code: string) => Promise<string[]>;
  disableTwoFactor: (code: string) => Promise<void>;
  regenerateRecoveryCodes: (code: string) => Promise<string[]>;
}

export interface SecuritySettingsController extends TwoFactorSettingsActions {
  settings: SecuritySettings | null;
  loading: boolean;
  error: string | null;
  setSingleSessionEnabled: (enabled: boolean) => Promise<void>;
  setHistoryEnabled: (enabled: boolean) => Promise<void>;
  setRecoveryCodeAlertEnabled: (enabled: boolean) => Promise<void>;
  acknowledgeAlert: () => Promise<void>;
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

  const beginTwoFactorEnrollment = useCallback(
    () => securityApi.beginTwoFactorEnrollment(),
    []
  );

  const confirmTwoFactorEnrollment = useCallback(
    async (enrollmentToken: string, code: string) => {
      const recoveryCodes = await securityApi.confirmTwoFactorEnrollment(enrollmentToken, code);
      await refresh();
      return recoveryCodes;
    },
    [refresh]
  );

  const disableTwoFactor = useCallback(
    async (code: string) => {
      await securityApi.disableTwoFactor(code);
      await refresh();
    },
    [refresh]
  );

  const regenerateRecoveryCodes = useCallback(
    async (code: string) => {
      const recoveryCodes = await securityApi.regenerateRecoveryCodes(code);
      await refresh();
      return recoveryCodes;
    },
    [refresh]
  );

  const updatePreferences = useCallback(async (input: SecurityPreferencesInput) => {
    setSettings(await securityApi.updatePreferences(input));
  }, []);

  const setSingleSessionEnabled = useCallback(
    (enabled: boolean) => updatePreferences({ singleSessionEnabled: enabled }),
    [updatePreferences]
  );

  const setHistoryEnabled = useCallback(
    (enabled: boolean) => updatePreferences({ historyEnabled: enabled }),
    [updatePreferences]
  );

  const setRecoveryCodeAlertEnabled = useCallback(
    (enabled: boolean) => updatePreferences({ recoveryCodeAlertEnabled: enabled }),
    [updatePreferences]
  );

  const acknowledgeAlert = useCallback(async () => {
    await securityApi.acknowledgeAlert();
    setSettings((current) => (current ? { ...current, securityAlert: undefined } : current));
  }, []);

  return {
    settings,
    loading,
    error,
    beginTwoFactorEnrollment,
    confirmTwoFactorEnrollment,
    disableTwoFactor,
    regenerateRecoveryCodes,
    setSingleSessionEnabled,
    setHistoryEnabled,
    setRecoveryCodeAlertEnabled,
    acknowledgeAlert,
  };
}
