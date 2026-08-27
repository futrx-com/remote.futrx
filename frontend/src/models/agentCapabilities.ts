import type { ChatProvider } from "./chat";

export interface AgentCapabilityOption {
  value: string;
  label: string;
  description?: string;
}

export interface AgentModelCapability {
  id: string;
  label: string;
  description?: string;
  providerDefault?: boolean;
  reasoningEfforts: AgentCapabilityOption[];
  defaultReasoningEffort?: string;
  serviceTiers: AgentCapabilityOption[];
  defaultServiceTier?: string;
}

export interface AgentProviderCapabilities {
  provider: ChatProvider;
  label: string;
  default?: boolean;
  executionScopes?: Array<"host" | "project">;
  authentication?: {
    mode: "managed-code" | "managed-device" | "external" | "none";
    instructions?: string;
    satisfiesAccessGate: boolean;
  };
  features?: {
    sessions: { resume: boolean; fork: boolean };
    skills: "none" | "slash-command" | "dollar-mention" | "instructions";
    browserTools: boolean;
    scheduledTools: boolean;
  };
  version?: string;
  source: "live" | "fallback";
  warning?: string;
  unavailableReason?: string;
  models: AgentModelCapability[];
  modes: AgentCapabilityOption[];
  defaultMode?: string;
}

export interface AgentCapabilitiesCatalog {
  providers: AgentProviderCapabilities[];
}
