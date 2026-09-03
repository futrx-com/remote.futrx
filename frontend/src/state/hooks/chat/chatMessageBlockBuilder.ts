import type { ChatEvent } from "../../../models/chat";
import type {
  AssistantMessageBlock,
  AssistantMessagePart,
  ChatMessageBlock,
} from "../../../models/chatMessage";

type AssistantToolPart = Extract<AssistantMessagePart, { kind: "tool" }>;

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
        assistant.parts.push({ kind: "thinking", text: event.text });
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
      case "complete":
        return this.endTrailingAssistant(blocks);
      case "error": {
        const next = this.endTrailingAssistant(blocks);
        return [...next, { type: "error", message: event.message, t: event.t }];
      }
      default:
        return blocks;
    }
  }

  private endTrailingAssistant(blocks: ChatMessageBlock[]): ChatMessageBlock[] {
    const lastIndex = blocks.length - 1;
    const last = blocks[lastIndex];
    if (!last || last.type !== "assistant" || last.isComplete) return blocks;
    const next = blocks.slice();
    next[lastIndex] = { ...last, isComplete: true };
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
}

export const chatMessageBlockBuilder = new ChatMessageBlockBuilder();
