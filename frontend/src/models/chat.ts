import type { ChatMessageBlock } from "./chatMessage";
import type { ChatUsagePayload, ChatUsageTotals } from "./chatUsage";

// Provider identifiers come from the backend module catalog. Built-in string
// literals remain valid, but future modules do not require a frontend type edit.
export type ChatProvider = string;
export type ChatMode = string;
export type ReasoningEffort = string;
export type ServiceTier = string;
export type ApprovalPolicy = "untrusted" | "on-request" | "never";
export type SandboxPolicy = "readOnly" | "workspaceWrite" | "dangerFullAccess";

export interface ChatMeta {
  id: string;
  title: string;
  provider?: ChatProvider;
  sessions?: Record<string, string>;
  claudeSessionId?: string;
  codexSessionId?: string;
  kimiSessionId?: string;
  antigravitySessionId?: string;
  tmuxSession?: string;
  cwd?: string;
  createdAt: number;
  lastMessageAt: number;
  lastReadAt?: number;
  running?: boolean;
  model?: string;
  mode?: ChatMode;
  reasoningEffort?: ReasoningEffort;
  serviceTier?: ServiceTier;
  approvalPolicy?: ApprovalPolicy;
  sandboxPolicy?: SandboxPolicy;
  projectId?: string;
  selectedSkills?: SelectedSkill[];
}

export interface SelectedSkill {
  name: string;
  command?: string;
  provider?: ChatProvider;
  source?: string;
}

export interface ProviderNativeEnvelope {
  schemaVersion: number;
  method: string;
  threadId?: string;
  turnId?: string;
  itemId?: string;
  requestId?: string;
  payload?: unknown;
}

type ChatEventBase = {
  seq?: number;
  t: number;
  turnId?: string;
  native?: ProviderNativeEnvelope;
  provider?: ChatProvider;
  status?: string;
};

export type ChatEvent = ChatEventBase & (
  | { type: "user"; text: string }
  | { type: "assistant_text"; text: string; messageId?: string }
  | { type: "thinking"; text: string; messageId?: string }
  | { type: "tool_use_start"; id: string; name: string; input: Record<string, unknown> }
  | { type: "tool_use_end"; id: string; output?: string; isError?: boolean }
  | { type: "permission_request"; id: string; toolName: string; input: Record<string, unknown> }
  | { type: "interaction_request"; id: string; interactionId?: string; name: string; input?: Record<string, unknown> }
  | { type: "interaction_resolved"; id: string; interactionId?: string; name?: string }
  | { type: "collaboration"; id: string; name?: string; data?: Record<string, unknown> }
  | { type: "turn_status"; data?: Record<string, unknown> }
  | { type: "provider_event"; name?: string; data?: unknown }
  | { type: "usage_update"; usage?: ChatUsagePayload }
  | { type: "system"; subtype: string; data?: Record<string, unknown> }
  | { type: "session"; sessionId?: string; claudeSessionId?: string; codexSessionId?: string; kimiSessionId?: string; antigravitySessionId?: string }
  | { type: "complete"; usage?: ChatUsagePayload }
  | { type: "error"; message: string }
  | { type: "sync"; running?: boolean }
);

export interface ChatEventPage {
  events: ChatEvent[];
  nextBefore?: number;
  lastSeq: number;
  hasMore: boolean;
}

export type ClientToServer =
  | { type: "prompt"; text: string; clientId?: string }
  | { type: "cancel" }
  | { type: "interaction_response"; interactionId: string; result?: unknown; error?: unknown }
  | { type: "permission"; id: string; approved: boolean };

export type ChatStatus = "loading" | "ready" | "streaming" | "error";

export interface QueuedPrompt {
  id: string;
  text: string;
}

export type ComposerSessionStorage = Pick<Storage, "getItem" | "setItem">;

export interface PersistedComposerSession {
  drafts: Record<string, string>;
  queues: Record<string, QueuedPrompt[]>;
}

export interface ChatComposerSessionStoreState {
  drafts: ReadonlyMap<string, string>;
  promptQueues: ReadonlyMap<string, QueuedPrompt[]>;
}

export interface ChatComposerSessionStoreActions {
  setDraft: (chatId: string, text: string) => void;
  setQueuedPrompts: (chatId: string, prompts: QueuedPrompt[]) => void;
}

// Server verdict on a prompt sent with a clientId: accepted means a run
// started from it; rejected means the run lock was held and it was discarded.
export interface PromptOutcome {
  clientId: string;
  accepted: boolean;
}

export interface CreateChatInput {
  tmuxSession?: string;
  cwd?: string;
  title?: string;
  provider?: ChatProvider;
  model?: string;
  mode?: ChatMode;
  reasoningEffort?: ReasoningEffort;
  serviceTier?: ServiceTier;
  approvalPolicy?: ApprovalPolicy;
  sandboxPolicy?: SandboxPolicy;
  projectId?: string;
  selectedSkills?: SelectedSkill[];
}

export interface UpdateChatInput {
  title?: string;
  cwd?: string;
  provider?: ChatProvider;
  model?: string;
  mode?: ChatMode;
  reasoningEffort?: ReasoningEffort;
  serviceTier?: ServiceTier;
  approvalPolicy?: ApprovalPolicy;
  sandboxPolicy?: SandboxPolicy;
  selectedSkills?: SelectedSkill[];
}

/** A chat's transcript as the thread renders it, plus where the next older
 *  page starts. */
export interface ChatRenderState {
  events: ChatEvent[];
  blocks: ChatMessageBlock[];
  usageTotals: ChatUsageTotals;
  eventCount: number;
  hasOlder: boolean;
  nextBefore: number;
}

/** A chat with every agent preference settled against the loaded detail and
 *  the account defaults, so no reader has to repeat the fallback chain. */
export interface ResolvedChatMeta extends ChatMeta {
  provider: ChatProvider;
  model: string;
  mode: ChatMode;
  reasoningEffort: ReasoningEffort;
  serviceTier: ServiceTier;
  approvalPolicy: ApprovalPolicy;
  sandboxPolicy: SandboxPolicy;
}
