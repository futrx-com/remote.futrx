import { useCallback, useEffect, useMemo, useState } from "preact/hooks";
import { agentAuthApi } from "../../../api/agents/auth/agentAuthApi";
import type {
  AgentAuthLoginSnapshot,
  AgentAuthProvider,
} from "../../../models/auth";
import { agentAuthRegistryService } from "../../../services/auth/agentAuthRegistryService.ts";

export interface AgentAuthRegistryState {
  providers: AgentAuthProvider[];
  loading: boolean;
  checked: boolean;
  gateReady: boolean;
  error: string | null;
  starting: Readonly<Record<string, boolean>>;
  actionErrors: Readonly<Record<string, string>>;
  startCodeLogin: (provider: string) => Promise<void>;
  submitCode: (provider: string, code: string) => Promise<void>;
  cancelCodeLogin: (provider: string) => Promise<void>;
  startDeviceLogin: (provider: string) => Promise<void>;
  saveAPIKey: (provider: string, apiKey: string) => Promise<boolean>;
  deleteAPIKey: (provider: string) => Promise<boolean>;
}

export function useAgentAuthRegistry(enabled: boolean): AgentAuthRegistryState {
  ////////////////
  // Local State
  ////////////////
  const [providers, setProviders] = useState<AgentAuthProvider[]>([]);
  const [loading, setLoading] = useState(false);
  const [checked, setChecked] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [starting, setStarting] = useState<Record<string, boolean>>({});
  const [actionErrors, setActionErrors] = useState<Record<string, string>>({});

  ////////////////
  // Handlers
  ////////////////
  // Every writer below reaches state through a setState updater, never through
  // the closure, so none of them can go stale and none needs dependencies.
  const setProviderStarting = useCallback((provider: string, value: boolean) => {
    setStarting((current) => ({ ...current, [provider]: value }));
  }, []);

  const setProviderError = useCallback((provider: string, value: string) => {
    setActionErrors((current) => ({ ...current, [provider]: value }));
  }, []);

  const updateLogin = useCallback((
    provider: string,
    login: AgentAuthLoginSnapshot,
    replace = false,
  ) => {
    setProviders((current) => {
      const entry = current.find((candidate) => candidate.provider === provider);
      if (!entry) return current;
      return agentAuthRegistryService.updateProvider(current, provider, {
        ...entry.status,
        login: replace ? login : { ...entry.status.login, ...login },
      });
    });
  }, []);

  const runAction = useCallback(async (provider: string, action: () => Promise<void>) => {
    setProviderStarting(provider, true);
    setProviderError(provider, "");
    try {
      await action();
      return true;
    } catch (caught) {
      setProviderError(provider, (caught as Error).message);
      return false;
    } finally {
      setProviderStarting(provider, false);
    }
  }, [setProviderStarting, setProviderError]);

  const startCodeLogin = useCallback(async (provider: string) => {
    await runAction(provider, async () => {
      const result = await agentAuthApi.startCodeLogin(provider);
      updateLogin(provider, {
        active: true,
        url: result.url,
        awaitingCode: true,
      });
    });
  }, [runAction, updateLogin]);

  const submitCode = useCallback(async (provider: string, code: string) => {
    await runAction(provider, async () => {
      await agentAuthApi.submitCode(provider, code);
    });
  }, [runAction]);

  const cancelCodeLogin = useCallback(async (provider: string) => {
    await runAction(provider, async () => {
      await agentAuthApi.cancelCodeLogin(provider);
      updateLogin(provider, { active: false }, true);
    });
  }, [runAction, updateLogin]);

  const startDeviceLogin = useCallback(async (provider: string) => {
    await runAction(provider, async () => {
      updateLogin(provider, await agentAuthApi.startDeviceLogin(provider), true);
    });
  }, [runAction, updateLogin]);

  const saveAPIKey = useCallback(async (provider: string, apiKey: string) =>
    runAction(provider, async () => {
      const status = await agentAuthApi.saveAPIKey(provider, apiKey);
      setProviders((current) => agentAuthRegistryService.updateProvider(current, provider, status));
    }), [runAction]);

  const deleteAPIKey = useCallback(async (provider: string) =>
    runAction(provider, async () => {
      const status = await agentAuthApi.deleteAPIKey(provider);
      setProviders((current) => agentAuthRegistryService.updateProvider(current, provider, status));
    }), [runAction]);

  ////////////////
  // Effects
  ////////////////
  useEffect(() => {
    if (!enabled) {
      setProviders([]);
      setLoading(false);
      setChecked(false);
      setError(null);
      setStarting({});
      setActionErrors({});
      return;
    }

    let cancelled = false;
    const subscriptions: Array<() => void> = [];
    setLoading(true);
    agentAuthApi.fetchCatalog()
      .then((catalog) => {
        if (cancelled) return;
        setProviders(catalog.providers);
        setError(null);
        setChecked(true);
        setLoading(false);
        for (const entry of catalog.providers) {
          if (
            entry.authentication.mode !== "managed-code"
            && entry.authentication.mode !== "managed-device"
            && entry.authentication.mode !== "managed-api-key"
          ) continue;
          subscriptions.push(agentAuthApi.subscribe(entry.provider, (status) => {
            if (cancelled) return;
            setProviders((current) =>
              agentAuthRegistryService.updateProvider(current, entry.provider, status));
          }));
        }
      })
      .catch((caught) => {
        if (cancelled) return;
        setError((caught as Error).message);
        setChecked(true);
        setLoading(false);
      });

    return () => {
      cancelled = true;
      for (const unsubscribe of subscriptions) unsubscribe();
    };
  }, [enabled]);

  return useMemo(() => ({
    providers,
    loading,
    checked,
    gateReady: agentAuthRegistryService.gateReady(providers),
    error,
    starting,
    actionErrors,
    startCodeLogin,
    submitCode,
    cancelCodeLogin,
    startDeviceLogin,
    saveAPIKey,
    deleteAPIKey,
  }), [
    providers,
    loading,
    checked,
    error,
    starting,
    actionErrors,
    startCodeLogin,
    submitCode,
    cancelCodeLogin,
    startDeviceLogin,
    saveAPIKey,
    deleteAPIKey,
  ]);
}
