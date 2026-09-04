import type {
  ChatMode,
  ApprovalPolicy,
  ChatProvider,
  ReasoningEffort,
  ServiceTier,
  SandboxPolicy,
} from "../../../models/chat";

export interface ComposerPreferences {
  provider: ChatProvider;
  model: string;
  mode: ChatMode;
  reasoningEffort: ReasoningEffort;
  serviceTier: ServiceTier;
  approvalPolicy: ApprovalPolicy;
  sandboxPolicy: SandboxPolicy;
}

export interface ComposerPreferenceActions {
  changeAgent: (provider: ChatProvider, model: string) => void;
  changeMode: (mode: ChatMode, modelPreset?: string, reasoningPreset?: string) => void;
  changeReasoningEffort: (reasoningEffort: ReasoningEffort) => void;
  changeServiceTier: (serviceTier: ServiceTier) => void;
  changeApprovalPolicy: (approvalPolicy: ApprovalPolicy) => void;
  changeSandboxPolicy: (sandboxPolicy: SandboxPolicy) => void;
}
