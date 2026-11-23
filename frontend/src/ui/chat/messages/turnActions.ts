import type { AssistantMessageBlock, ChatMessageBlock } from "../../../models/chatMessage";

export interface AssistantTurnActions {
  /** The reply's text parts as markdown, for Copy. */
  text: string;
  /** The prompt that started the turn; Retry rewinds to it and sends it again. */
  prompt?: { t: number; text: string };
  /**
   * Where Branch cuts the history: the next prompt's timestamp. Undefined for
   * the last turn, whose branch copies the whole chat.
   */
  branchBeforeT?: number;
}

/**
 * Actions for the assistant block at `index`, or null when it should show
 * none: while it is still streaming, and on all but the last assistant block
 * of a turn, so a turn gets one row of actions.
 */
export function assistantTurnActions(
  blocks: ChatMessageBlock[],
  index: number,
  streaming: boolean,
): AssistantTurnActions | null {
  const block = blocks[index];
  if (!block || block.type !== "assistant" || streaming || !block.isComplete) return null;
  if (blocks[index + 1]?.type === "assistant") return null;

  const text = assistantText(block);
  if (!text) return null;

  let prompt: AssistantTurnActions["prompt"];
  for (let i = index - 1; i >= 0; i -= 1) {
    const candidate = blocks[i];
    if (candidate.type === "user") {
      prompt = { t: candidate.t, text: candidate.text };
      break;
    }
  }

  let branchBeforeT: number | undefined;
  for (let i = index + 1; i < blocks.length; i += 1) {
    const candidate = blocks[i];
    if (candidate.type === "user") {
      branchBeforeT = candidate.t;
      break;
    }
  }

  return { text, prompt, branchBeforeT };
}

export function assistantText(block: AssistantMessageBlock): string {
  return block.parts
    .flatMap((part) => (part.kind === "text" && part.text.trim() ? [part.text.trim()] : []))
    .join("\n\n");
}
