import type { AssistantMessageBlock } from "../../../models/chatMessage";
import { AssistantPartList } from "./AssistantPartList";
import { ThinkingIndicator } from "./ThinkingIndicator";
import type { ChatInteractionResponder } from "../../../types/chatApi";

export function AssistantMessage({
  block,
  streaming,
  chatId,
  cwd,
  onAnswerQuestion,
  onRespondInteraction,
}: {
  block: AssistantMessageBlock;
  streaming: boolean;
  chatId?: string;
  cwd?: string;
  onAnswerQuestion?: (text: string) => void;
  onRespondInteraction?: ChatInteractionResponder;
}) {
  const reasoningActive = block.parts.at(-1)?.kind === "thinking";

  return (
    <div class="codex-assistant-block min-w-0 space-y-2 max-w-full">
      <AssistantPartList
        parts={block.parts}
        streaming={streaming}
        chatId={chatId}
        cwd={cwd}
        onAnswerQuestion={onAnswerQuestion}
        onRespondInteraction={onRespondInteraction}
      />
      {streaming && !block.isComplete && !reasoningActive && <ThinkingIndicator />}
    </div>
  );
}
