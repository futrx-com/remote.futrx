import type {
  AgentCapabilitiesCatalog,
  CapabilityPreferenceCorrection,
  CapabilityPreferenceSelection,
  ComposerCapabilityState,
  ComposerProviderOption,
} from "../../../models/agentCapabilities";
import type { ChatProvider } from "../../../models/chat";

// Resolves the composer's view of what the selected agent can do: which
// providers and models to offer, and which of the saved preferences the live
// catalog still supports.
class AgentCapabilityState {
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
      label: this.providerLabel(provider),
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
  }

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
      const replacement = this.supportedMode(state, capabilities.defaultMode);
      if (replacement !== undefined) correction.mode = replacement;
    }
    return correction;
  }

  // A replacement the provider actually offers, or undefined when it offers
  // nothing usable. The correction has to satisfy the same check that produced
  // it: naming a mode outside modeOptions leaves the selection invalid, so the
  // next pass corrects it again, and the composer never settles.
  //
  // Effort and tier cannot loop this way — they correct to "", which the
  // `selection.x &&` guard treats as nothing to correct.
  private supportedMode(
    state: ComposerCapabilityState,
    defaultMode: string | undefined,
  ): string | undefined {
    const offers = (value: string | undefined) =>
      value !== undefined && state.modeOptions.some((option) => option.value === value);
    if (offers(defaultMode)) return defaultMode;
    return state.modeOptions[0]?.value;
  }

  private providerLabel(provider: string): string {
    return provider ? provider.charAt(0).toUpperCase() + provider.slice(1) : "Agent";
  }
}

export const agentCapabilityState = new AgentCapabilityState();
