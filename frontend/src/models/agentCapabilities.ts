import type { ChatProvider } from "./chat";

export interface AgentCapabilityOption {
  value: string;
  label: string;
  description?: string;
  model?: string;
  reasoningEffort?: string;
  raw?: unknown;
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
  inputModalities?: string[];
  supportsPersonality?: boolean;
  multiAgentVersion?: string;
  hidden?: boolean;
  modelSpecialty?: string;
  upgrade?: string;
  upgradeInfo?: unknown;
  availabilityNux?: unknown;
  raw?: unknown;
}

export interface AgentProviderCapabilities {
  provider: ChatProvider;
  label: string;
  default?: boolean;
  executionScopes?: Array<"host" | "project">;
  authentication?: {
    mode: "managed-code" | "managed-device" | "managed-api-key" | "external" | "none";
    instructions?: string;
    satisfiesAccessGate: boolean;
    apiKey?: {
      createUrl: string;
      createLabel: string;
      credentialLabel: string;
    };
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

/** What one browser currently knows about a catalog scope. */
export interface AgentCapabilityCatalogSnapshot {
  catalog: AgentCapabilitiesCatalog | null;
  loading: boolean;
  refreshing: boolean;
  error: string;
}

/** A scope some part of the app is currently watching. */
export interface ObservedAgentCapabilityScope {
  /** Normalized, matching the user half of the scope's key. */
  userId: string;
  /** Empty for the host scope, matching the project half of the key. */
  projectId: string;
  observers: number;
}

export type AgentCapabilityCatalogRequester = (
  projectId?: string,
  options?: { refresh?: boolean },
) => Promise<AgentCapabilitiesCatalog>;

export interface AgentCapabilityCatalogLoadOptions {
  force?: boolean;
}

export interface AgentCapabilityCatalogStoreState {
  scopes: ReadonlyMap<string, AgentCapabilityCatalogSnapshot>;
}

export interface AgentCapabilityCatalogStoreActions {
  observe: (userId: string, projectId?: string) => () => void;
  load: (
    userId: string,
    projectId?: string,
    options?: AgentCapabilityCatalogLoadOptions,
  ) => Promise<AgentCapabilitiesCatalog>;
  invalidateUser: (userId: string) => void;
  removeProject: (userId: string, projectId: string) => void;
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

/** The composer's view of what the selected agent can do: which providers
 *  and models to offer, and which options the selected model supports. */
export interface ComposerCapabilityState {
  providerCapabilities?: AgentProviderCapabilities;
  providerOptions: ComposerProviderOption[];
  modelOptions: ComposerModelOption[];
  reasoningEffortOptions: AgentCapabilityOption[];
  serviceTierOptions: AgentCapabilityOption[];
  modeOptions: AgentCapabilityOption[];
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
