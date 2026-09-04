import type { ChatEvent } from "../../../models/chat";
import type {
  AssistantMessageBlock,
  AssistantMessagePart,
  ChatMessageBlock,
} from "../../../models/chatMessage";
import { chatInteractionService } from "../../../services/chat/chatInteractionService.ts";

type AssistantToolPart = Extract<AssistantMessagePart, { kind: "tool" }>;
type AssistantInteractionPart = Extract<AssistantMessagePart, { kind: "interaction" }>;

// Folds a chat's event stream into the blocks the transcript renders: user
// turns, assistant turns built up part by part, and errors. Separate from the
// projector because this changes with the message model — a new part kind, a
// new tool state — while paging and merging do not.
class ChatMessageBlockBuilder {
  fromEvents(events: ChatEvent[]): ChatMessageBlock[] {
    return events.reduce<ChatMessageBlock[]>((blocks, event) => this.append(blocks, event), []);
  }

  private append(blocks: ChatMessageBlock[], event: ChatEvent): ChatMessageBlock[] {
    switch (event.type) {
      case "user": {
        const next = this.endTrailingAssistant(blocks);
        return [...next, { type: "user", text: event.text, t: event.t }];
      }
      case "assistant_text": {
        const { blocks: next, assistant } = this.ensureTrailingAssistant(blocks, event.t);
        const lastIndex = assistant.parts.length - 1;
        const last = assistant.parts[lastIndex];
        if (last?.kind === "text") {
          assistant.parts[lastIndex] = { ...last, text: last.text + event.text };
        } else {
          assistant.parts.push({ kind: "text", text: event.text });
        }
        return next;
      }
      case "thinking": {
        const { blocks: next, assistant } = this.ensureTrailingAssistant(blocks, event.t);
        const lastIndex = assistant.parts.length - 1;
        const last = assistant.parts[lastIndex];
        if (last?.kind === "thinking") {
          assistant.parts[lastIndex] = { ...last, text: last.text + event.text };
        } else {
          assistant.parts.push({ kind: "thinking", text: event.text });
        }
        return next;
      }
      case "tool_use_start": {
        const { blocks: next, assistant } = this.ensureTrailingAssistant(blocks, event.t);
        assistant.parts.push({
          kind: "tool",
          id: event.id,
          name: event.name,
          input: event.input ?? {},
          status: "running",
        });
        return next;
      }
      case "tool_use_end":
        return this.updateTrailingTool(blocks, event.id, {
          output: event.output,
          isError: event.isError,
          status: "done",
        });
      case "interaction_request": {
        const { blocks: next, assistant } = this.ensureTrailingAssistant(blocks, event.t);
        assistant.parts.push({
          kind: "interaction",
          id: event.id,
          method: event.name,
          input: event.input ?? {},
          interactionKind: event.status || "provider_request",
          supportsCancellation: chatInteractionService.supportsApprovalCancellation(event.name),
          status: "pending",
        });
        return next;
      }
      case "interaction_resolved":
        return this.updateInteraction(blocks, event.id, { status: event.status || "resolved" });
      case "collaboration": {
        // wait is an internal parent/subagent synchronization primitive. Its
        // native event stays in the transcript log, while child-thread updates
        // provide the user-facing status and report cards.
        if (event.name === "wait") return blocks;
        const { blocks: next, assistant } = this.ensureTrailingAssistant(blocks, event.t);
        const existing = assistant.parts.findIndex(
          (part) => part.kind === "collaboration" && part.id === event.id,
        );
        const part: AssistantMessagePart = {
          kind: "collaboration",
          id: event.id,
          name: event.name,
          data: event.data ?? {},
          status: event.status || "inProgress",
        };
        if (existing >= 0) assistant.parts[existing] = part;
        else assistant.parts.push(part);
        return next;
      }
      case "turn_status": {
        const { blocks: next, assistant } = this.ensureTrailingAssistant(blocks, event.t);
        const existing = assistant.parts.findIndex((part) => part.kind === "turn-status");
        const part: AssistantMessagePart = {
          kind: "turn-status",
          status: event.status || "unknown",
          data: event.data,
          ...(event.provider ? { provider: event.provider } : {}),
        };
        if (existing >= 0) assistant.parts[existing] = part;
        else assistant.parts.push(part);
        if (["completed", "failed", "interrupted"].includes(part.status)) {
          return this.endTrailingAssistant(next);
        }
        return next;
      }
      // Provider-native fallback events stay in the persisted event stream for
      // diagnostics and future typed projections. They are protocol telemetry,
      // not conversation content, so the normal transcript does not render them.
      case "provider_event":
        return blocks;
      case "complete":
        return this.endTrailingAssistant(this.updateTrailingTurnStatus(
          blocks,
          event.status || "completed",
          event.provider,
        ));
      case "error": {
        const withStatus = event.status === "failed"
          ? this.updateTrailingTurnStatus(blocks, "failed", event.provider)
          : blocks;
        const next = this.endTrailingAssistant(withStatus, "failed");
        return [...next, { type: "error", message: event.message, t: event.t }];
      }
      default:
        return blocks;
    }
  }

  private endTrailingAssistant(
    blocks: ChatMessageBlock[],
    unfinishedCollaborationStatus = "turnEnded",
  ): ChatMessageBlock[] {
    const lastIndex = blocks.length - 1;
    const last = blocks[lastIndex];
    if (!last || last.type !== "assistant" || last.isComplete) return blocks;
    const next = blocks.slice();
    const parts: AssistantMessagePart[] = last.parts.map((part) => {
      if (part.kind === "tool" && part.status === "running") {
        return { ...part, status: "done" };
      }
      if (part.kind === "collaboration" && part.status === "inProgress") {
        return { ...part, status: unfinishedCollaborationStatus };
      }
      return part;
    });
    next[lastIndex] = { ...last, parts, isComplete: true };
    return next;
  }

  private ensureTrailingAssistant(
    blocks: ChatMessageBlock[],
    timestamp: number
  ): { blocks: ChatMessageBlock[]; assistant: AssistantMessageBlock } {
    const lastIndex = blocks.length - 1;
    const last = blocks[lastIndex];
    if (last?.type === "assistant" && !last.isComplete) {
      const next = blocks.slice();
      const assistant: AssistantMessageBlock = { ...last, parts: last.parts.slice() };
      next[lastIndex] = assistant;
      return { blocks: next, assistant };
    }

    const assistant: AssistantMessageBlock = {
      type: "assistant",
      parts: [],
      t: timestamp,
      isComplete: false,
    };
    return { blocks: [...blocks, assistant], assistant };
  }

  private updateTrailingTool(
    blocks: ChatMessageBlock[],
    id: string,
    patch: Partial<AssistantToolPart>
  ): ChatMessageBlock[] {
    const lastIndex = blocks.length - 1;
    const last = blocks[lastIndex];
    if (!last || last.type !== "assistant") return blocks;

    const partIndex = last.parts.findIndex((part) => part.kind === "tool" && part.id === id);
    if (partIndex < 0) return blocks;
    const part = last.parts[partIndex];
    if (part.kind !== "tool") return blocks;

    const next = blocks.slice();
    const parts = last.parts.slice();
    parts[partIndex] = { ...part, ...patch };
    next[lastIndex] = { ...last, parts };
    return next;
  }

  private updateInteraction(
    blocks: ChatMessageBlock[],
    id: string,
    patch: Partial<AssistantInteractionPart>,
  ): ChatMessageBlock[] {
    for (let blockIndex = blocks.length - 1; blockIndex >= 0; blockIndex--) {
      const block = blocks[blockIndex];
      if (block.type !== "assistant") continue;
      const partIndex = block.parts.findIndex(
        (part) => part.kind === "interaction" && part.id === id,
      );
      if (partIndex < 0) continue;
      const part = block.parts[partIndex];
      if (part.kind !== "interaction") return blocks;
      const next = blocks.slice();
      const parts = block.parts.slice();
      parts[partIndex] = { ...part, ...patch };
      next[blockIndex] = { ...block, parts };
      return next;
    }
    return blocks;
  }

  private updateTrailingTurnStatus(
    blocks: ChatMessageBlock[],
    status: string,
    provider?: string,
  ): ChatMessageBlock[] {
    const lastIndex = blocks.length - 1;
    const last = blocks[lastIndex];
    if (!last || last.type !== "assistant") return blocks;
    const partIndex = last.parts.findIndex((part) => part.kind === "turn-status");
    if (partIndex < 0) return blocks;
    const part = last.parts[partIndex];
    if (part.kind !== "turn-status") return blocks;
    const next = blocks.slice();
    const parts = last.parts.slice();
    parts[partIndex] = {
      ...part,
      status,
      ...(provider ? { provider } : {}),
    };
    next[lastIndex] = { ...last, parts };
    return next;
  }
}

export const chatMessageBlockBuilder = new ChatMessageBlockBuilder();
