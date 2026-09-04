import type {
  ApprovalPolicy,
  ChatMode,
  ChatProvider,
  ReasoningEffort,
  SandboxPolicy,
  ServiceTier,
} from "./chat";

export type AppearanceTheme = "system" | "dark" | "light";

export interface AppearanceSettings {
  theme: AppearanceTheme;
}

export interface ChatSettings {
  provider: ChatProvider;
  model: string;
  mode: ChatMode;
  reasoningEffort: ReasoningEffort;
  serviceTier: ServiceTier;
  approvalPolicy: ApprovalPolicy;
  sandboxPolicy: SandboxPolicy;
}

export interface UserSettings {
  appearance: AppearanceSettings;
  /** Preferences for loose chats running on the host. */
  chat: ChatSettings;
  /** Preferences for chats running inside a project container. */
  projectChat: ChatSettings;
  updatedAt?: number;
}

export interface UpdateUserSettingsInput {
  appearance?: Partial<AppearanceSettings>;
  chat?: Partial<ChatSettings>;
  projectChat?: Partial<ChatSettings>;
}
