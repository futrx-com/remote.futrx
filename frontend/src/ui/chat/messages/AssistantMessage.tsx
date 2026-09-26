import type { AssistantMessageBlock } from "../../../models/chatMessage";
import { AssistantPartList } from "./AssistantPartList";
import { hasVisibleAssistantContent } from "./turnActivity";
import type { ChatInteractionResponder } from "../../../types/chatApi";

export function AssistantMessage({
  block,
  hydratedPartIndex,
  streaming,
  chatId,
  cwd,
  onAnswerQuestion,
  onRespondInteraction,
  streamingPresentation,
}: {
  block: AssistantMessageBlock;
  hydratedPartIndex?: number;
  streaming: boolean;
  chatId?: string;
  cwd?: string;
  onAnswerQuestion?: (text: string) => void;
  onRespondInteraction?: ChatInteractionResponder;
  streamingPresentation: "blocks" | "tokens";
}) {
  if (!hasVisibleAssistantContent(block)) return null;
  return (
    <div class="codex-assistant-block min-w-0 space-y-2 max-w-full">
      <AssistantPartList
        parts={block.parts}
        hydratedPartIndex={hydratedPartIndex}
        streaming={streaming && !block.isComplete}
        streamingPresentation={streamingPresentation}
        chatId={chatId}
        cwd={cwd}
        onAnswerQuestion={onAnswerQuestion}
        onRespondInteraction={onRespondInteraction}
      />
    </div>
  );
}
