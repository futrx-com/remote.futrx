import type {
  ChatMode,
  ChatProvider,
  ReasoningEffort,
  ServiceTier,
} from "../../../models/chat";

export interface ComposerPreferences {
  provider: ChatProvider;
  model: string;
  mode: ChatMode;
  reasoningEffort: ReasoningEffort;
  serviceTier: ServiceTier;
}

export interface ComposerPreferenceActions {
  changeAgent: (provider: ChatProvider, model: string) => void;
  changeMode: (mode: ChatMode) => void;
  changeReasoningEffort: (reasoningEffort: ReasoningEffort) => void;
  changeServiceTier: (serviceTier: ServiceTier) => void;
}
