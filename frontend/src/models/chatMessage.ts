import type { ChatProvider } from "./chat";

export type AssistantMessagePart =
  | { kind: "text"; text: string }
  | {
      kind: "tool";
      id: string;
      name: string;
      input: Record<string, unknown>;
      output?: string;
      isError?: boolean;
      status: "running" | "done";
    }
  | { kind: "thinking"; text: string }
  | {
      kind: "interaction";
      id: string;
      method: string;
      input: Record<string, unknown>;
      interactionKind: string;
      supportsCancellation: boolean;
      status: string;
    }
  | {
      kind: "collaboration";
      id: string;
      name?: string;
      data: Record<string, unknown>;
      status: string;
    }
  | { kind: "turn-status"; status: string; data?: Record<string, unknown>; provider?: ChatProvider };

export type AssistantMessageBlock = {
  type: "assistant";
  parts: AssistantMessagePart[];
  t: number;
  isComplete: boolean;
};

export type ChatMessageBlock =
  | { type: "user"; text: string; t: number }
  | AssistantMessageBlock
  | { type: "error"; message: string; t: number };
