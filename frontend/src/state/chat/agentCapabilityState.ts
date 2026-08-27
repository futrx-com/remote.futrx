import type {
  AgentCapabilitiesCatalog,
  AgentCapabilityOption,
  AgentProviderCapabilities,
} from "../../models/agentCapabilities";
import type { ChatProvider } from "../../models/chat";

export interface ComposerCapabilityState {
  providerCapabilities?: AgentProviderCapabilities;
  providerOptions: ComposerProviderOption[];
  modelOptions: ComposerModelOption[];
  reasoningEffortOptions: AgentCapabilityOption[];
  serviceTierOptions: AgentCapabilityOption[];
  modeOptions: AgentCapabilityOption[];
}

export interface ComposerModelOption {
  value: string;
  label: string;
  sub: string;
}

export interface ComposerProviderOption {
  value: ChatProvider;
  label: string;
  disabled?: boolean;
  disabledReason?: string;
  models: ComposerModelOption[];
}

export interface CapabilityPreferenceSelection {
  mode: string;
  reasoningEffort: string;
  serviceTier: string;
}

export interface CapabilityPreferenceCorrection {
  mode?: string;
  reasoningEffort?: string;
  serviceTier?: string;
}

export const agentCapabilityState = {
  resolve(
    catalog: AgentCapabilitiesCatalog | null,
    provider: ChatProvider,
    model: string,
    loading: boolean,
    unavailableProviders: Partial<Record<ChatProvider, string>> = {},
  ): ComposerCapabilityState {
    const providerCapabilities = catalog?.providers.find(
      (item) => item.provider === provider,
    );
    const discoveredProviderOptions = catalog?.providers.map((item) => ({
      value: item.provider,
      label: item.label,
      disabled: !!unavailableProviders[item.provider],
      disabledReason: unavailableProviders[item.provider],
      models: item.models.map((modelItem) => ({
        value: modelItem.id,
        label: modelItem.label,
        sub: modelItem.description
          || (modelItem.providerDefault ? "provider default" : "available model"),
      })),
    })) ?? [];
    const savedProviderFallback: ComposerProviderOption = {
      value: provider,
      label: providerLabel(provider),
      disabled: !!unavailableProviders[provider],
      disabledReason: unavailableProviders[provider],
      models: loading ? [] : [{
        value: model,
        label: model || "Auto",
        sub: "current selection",
      }],
    };
    const providerOptions: ComposerProviderOption[] = discoveredProviderOptions.some(
      (option) => option.value === provider,
    )
      ? discoveredProviderOptions
      : [...discoveredProviderOptions, savedProviderFallback];
    const modelOptions = providerOptions.find((option) => option.value === provider)?.models ?? [];
    const selectedModel = providerCapabilities?.models.find((item) => item.id === model)
      ?? providerCapabilities?.models.find((item) => item.id === "");
    return {
      providerCapabilities,
      providerOptions,
      modelOptions,
      reasoningEffortOptions: selectedModel?.reasoningEfforts ?? [],
      serviceTierOptions: selectedModel?.serviceTiers ?? [],
      modeOptions: providerCapabilities?.modes ?? [],
    };
  },

  corrections(
    state: ComposerCapabilityState,
    selection: CapabilityPreferenceSelection,
  ): CapabilityPreferenceCorrection {
    const capabilities = state.providerCapabilities;
    if (!capabilities || capabilities.source !== "live") return {};

    const correction: CapabilityPreferenceCorrection = {};
    if (
      selection.reasoningEffort &&
      !state.reasoningEffortOptions.some(
        (option) => option.value === selection.reasoningEffort,
      )
    ) {
      correction.reasoningEffort = "";
    }
    if (
      selection.serviceTier &&
      !state.serviceTierOptions.some(
        (option) => option.value === selection.serviceTier,
      )
    ) {
      correction.serviceTier = "";
    }
    if (
      selection.mode &&
      !state.modeOptions.some((option) => option.value === selection.mode)
    ) {
      correction.mode = capabilities.defaultMode || state.modeOptions[0]?.value || "default";
    }
    return correction;
  },
};

function providerLabel(provider: string): string {
  return provider ? provider.charAt(0).toUpperCase() + provider.slice(1) : "Agent";
}
