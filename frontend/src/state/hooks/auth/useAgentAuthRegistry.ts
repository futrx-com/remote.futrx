import { useEffect, useState } from "preact/hooks";
import { agentAuthApi } from "../../../api/agents/auth/agentAuthApi";
import type {
  AgentAuthCatalog,
  AgentAuthLoginSnapshot,
  AgentAuthProvider,
  AgentAuthSnapshot,
} from "../../../models/auth";
import {
  agentAuthGateReady,
  updateAgentAuthProvider,
} from "../../auth/agentAuthRegistryState";

export interface AgentAuthRegistryState {
  providers: AgentAuthProvider[];
  loading: boolean;
  checked: boolean;
  gateReady: boolean;
  error: string | null;
  starting: Readonly<Record<string, boolean>>;
  actionErrors: Readonly<Record<string, string>>;
  refresh: () => Promise<void>;
  startCodeLogin: (provider: string) => Promise<void>;
  submitCode: (provider: string, code: string) => Promise<void>;
  cancelCodeLogin: (provider: string) => Promise<void>;
  startDeviceLogin: (provider: string) => Promise<void>;
}

export function useAgentAuthRegistry(enabled: boolean): AgentAuthRegistryState {
  const [providers, setProviders] = useState<AgentAuthProvider[]>([]);
  const [loading, setLoading] = useState(false);
  const [checked, setChecked] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [starting, setStarting] = useState<Record<string, boolean>>({});
  const [actionErrors, setActionErrors] = useState<Record<string, string>>({});

  function applyStatus(provider: string, status: AgentAuthSnapshot) {
    setProviders((current) => updateAgentAuthProvider(current, provider, status));
  }

  async function fetchCatalog(): Promise<AgentAuthCatalog> {
    const catalog = await agentAuthApi.fetchCatalog();
    setProviders(catalog.providers);
    setError(null);
    setChecked(true);
    return catalog;
  }

  async function refresh() {
    setLoading(true);
    try {
      await fetchCatalog();
    } catch (caught) {
      setError((caught as Error).message);
      setChecked(true);
    } finally {
      setLoading(false);
    }
  }

  function setProviderStarting(provider: string, value: boolean) {
    setStarting((current) => ({ ...current, [provider]: value }));
  }

  function setProviderError(provider: string, value: string) {
    setActionErrors((current) => ({ ...current, [provider]: value }));
  }

  function updateLogin(
    provider: string,
    login: AgentAuthLoginSnapshot,
    replace = false,
  ) {
    setProviders((current) => {
      const entry = current.find((candidate) => candidate.provider === provider);
      if (!entry) return current;
      return updateAgentAuthProvider(current, provider, {
        ...entry.status,
        login: replace ? login : { ...entry.status.login, ...login },
      });
    });
  }

  async function runAction(provider: string, action: () => Promise<void>) {
    setProviderStarting(provider, true);
    setProviderError(provider, "");
    try {
      await action();
    } catch (caught) {
      setProviderError(provider, (caught as Error).message);
    } finally {
      setProviderStarting(provider, false);
    }
  }

  async function startCodeLogin(provider: string) {
    await runAction(provider, async () => {
      const result = await agentAuthApi.startCodeLogin(provider);
      updateLogin(provider, {
        active: true,
        url: result.url,
        awaitingCode: true,
      });
    });
  }

  async function submitCode(provider: string, code: string) {
    await runAction(provider, async () => {
      await agentAuthApi.submitCode(provider, code);
    });
  }

  async function cancelCodeLogin(provider: string) {
    await runAction(provider, async () => {
      await agentAuthApi.cancelCodeLogin(provider);
      updateLogin(provider, { active: false }, true);
    });
  }

  async function startDeviceLogin(provider: string) {
    await runAction(provider, async () => {
      updateLogin(provider, await agentAuthApi.startDeviceLogin(provider), true);
    });
  }

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
          ) continue;
          subscriptions.push(agentAuthApi.subscribe(entry.provider, (status) => {
            if (!cancelled) applyStatus(entry.provider, status);
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

  return {
    providers,
    loading,
    checked,
    gateReady: agentAuthGateReady(providers),
    error,
    starting,
    actionErrors,
    refresh,
    startCodeLogin,
    submitCode,
    cancelCodeLogin,
    startDeviceLogin,
  };
}
