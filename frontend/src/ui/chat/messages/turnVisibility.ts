import type { ChatStatus } from "../../../models/chat";
import type { AssistantMessageBlock, ChatMessageBlock } from "../../../models/chatMessage";

export function showTerminalTurnStatus(status: string): boolean {
  return status === "failed";
}

export function hasVisibleAssistantContent(block: AssistantMessageBlock): boolean {
  return block.parts.some((part) => part.kind !== "turn-status" || showTerminalTurnStatus(part.status));
}

export function showTurnActivity(
  status: ChatStatus,
  blocks: ChatMessageBlock[],
  locallyStartedTurn: boolean,
): boolean {
  if (status !== "streaming") return false;
  if (locallyStartedTurn) return true;
  const last = blocks.at(-1);
  if (!last || last.type === "user") return true;
  if (last.type !== "assistant" || last.isComplete) return false;
  return last.parts.at(-1)?.kind !== "thinking";
}
