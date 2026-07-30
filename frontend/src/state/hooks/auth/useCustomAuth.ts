import { useEffect, useState } from "preact/hooks";
import type { CustomAuthStatus } from "../../../models/auth";
import { customAuthApi } from "../../../api/agents/auth/customAuthApi";

export interface CustomAuthState {
  loading: boolean;
  checked: boolean;
  authenticated: boolean;
  config?: { name: string; baseUrl: string };
  saving: boolean;
  error: string | null;
  save: (name: string, apiKey: string, baseUrl: string) => Promise<void>;
}

export function useCustomAuth(enabled: boolean): CustomAuthState {
  const [loading, setLoading] = useState(false);
  const [checked, setChecked] = useState(false);
  const [authenticated, setAuthenticated] = useState(false);
  const [config, setConfig] = useState<{ name: string; baseUrl: string } | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function applyStatus(status: CustomAuthStatus) {
    setAuthenticated(!!status.authenticated);
    setConfig(status.config);
    setError(null);
    setLoading(false);
    setChecked(true);
  }

  async function save(name: string, apiKey: string, baseUrl: string) {
    setSaving(true);
    setError(null);
    try {
      await customAuthApi.save({ name, apiKey, baseUrl });
    } catch (e) {
      setError((e as Error).message);
      throw e;
    } finally {
      setSaving(false);
    }
  }

  useEffect(() => {
    if (!enabled) {
      setLoading(false);
      setChecked(false);
      setAuthenticated(false);
      setConfig(undefined);
      setError(null);
      return;
    }

    setLoading(true);
    return customAuthApi.subscribe(applyStatus);
  }, [enabled]);

  return {
    loading,
    checked,
    authenticated,
    config,
    saving,
    error,
    save,
  };
}
