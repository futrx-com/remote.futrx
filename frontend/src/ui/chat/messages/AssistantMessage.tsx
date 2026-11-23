import type { AssistantMessageBlock } from "../../../models/chatMessage";
import { AssistantActions } from "./AssistantActions";
import { AssistantPartList } from "./AssistantPartList";
import type { AssistantTurnActions } from "./turnActions";
import { hasVisibleAssistantContent } from "./turnVisibility";
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
  actions,
  onRetry,
  onBranch,
}: {
  block: AssistantMessageBlock;
  hydratedPartIndex?: number;
  streaming: boolean;
  chatId?: string;
  cwd?: string;
  onAnswerQuestion?: (text: string) => boolean;
  onRespondInteraction?: ChatInteractionResponder;
  streamingPresentation: "blocks" | "tokens";
  actions?: AssistantTurnActions | null;
  onRetry?: (t: number, text: string) => void;
  onBranch?: (beforeT?: number) => void;
}) {
  if (!hasVisibleAssistantContent(block)) return null;
  return (
    <div class="codex-assistant-block group min-w-0 space-y-2 max-w-full">
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
      {actions && <AssistantActions actions={actions} onRetry={onRetry} onBranch={onBranch} />}
    </div>
  );
}
