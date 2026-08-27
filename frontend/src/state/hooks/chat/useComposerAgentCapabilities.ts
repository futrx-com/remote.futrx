import { useEffect } from "preact/hooks";
import type {
  ChatMode,
  ChatProvider,
  ReasoningEffort,
  ServiceTier,
} from "../../../models/chat";
import { agentCapabilityState } from "../../chat/agentCapabilityState";
import { unavailableManagedAgents } from "../../auth/agentAuthRegistryState";
import { useAuthContext } from "../../context/AuthContext";
import { useAgentCapabilities } from "./useAgentCapabilities";

interface CapabilityPreferenceActions {
  changeMode: (mode: ChatMode) => void;
  changeReasoningEffort: (reasoningEffort: ReasoningEffort) => void;
  changeServiceTier: (serviceTier: ServiceTier) => void;
}

export function useComposerAgentCapabilities({
  projectId,
  provider,
  model,
  mode,
  reasoningEffort,
  serviceTier,
  actions,
}: {
  projectId?: string;
  provider: ChatProvider;
  model: string;
  mode: ChatMode;
  reasoningEffort: ReasoningEffort;
  serviceTier: ServiceTier;
  actions: CapabilityPreferenceActions;
}) {
  const { agentAuth, providerAuthChecked } = useAuthContext();
  const capabilities = useAgentCapabilities(projectId);
  const unavailableProviders: Partial<Record<ChatProvider, string>> =
    unavailableManagedAgents(agentAuth.providers, providerAuthChecked);
  for (const item of capabilities.catalog?.providers ?? []) {
    if (item.unavailableReason) {
      unavailableProviders[item.provider] = item.unavailableReason;
    }
  }
  const state = agentCapabilityState.resolve(
    capabilities.catalog,
    provider,
    model,
    capabilities.loading,
    unavailableProviders,
  );

  useEffect(() => {
    const corrections = agentCapabilityState.corrections(state, {
      mode,
      reasoningEffort,
      serviceTier,
    });
    if (corrections.reasoningEffort !== undefined) {
      actions.changeReasoningEffort(corrections.reasoningEffort);
    }
    if (corrections.serviceTier !== undefined) {
      actions.changeServiceTier(corrections.serviceTier);
    }
    if (corrections.mode !== undefined) {
      actions.changeMode(corrections.mode);
    }
  }, [
    state.modeOptions,
    mode,
    model,
    reasoningEffort,
    serviceTier,
    state.providerCapabilities,
    state.reasoningEffortOptions,
    state.serviceTierOptions,
  ]);

  return {
    ...state,
    loading: capabilities.loading,
    refreshing: capabilities.refreshing,
    error: capabilities.error,
    refresh: capabilities.refresh,
  };
}
