import type { ChatMessageBlock } from "../../../models/chatMessage";
import { AssistantMessage } from "./AssistantMessage";
import { ErrorMessage } from "./ErrorMessage";
import { UserMessage } from "./UserMessage";
import type { ChatInteractionResponder } from "../../../types/chatApi";
import type { AssistantTurnActions } from "./turnActions";

export function MessageBlock({
  block,
  hydratedPartIndex,
  streaming,
  chatId,
  cwd,
  onAnswerQuestion,
  onRespondInteraction,
  onRewind,
  streamingPresentation,
  actions,
  onRetry,
  onBranch,
}: {
  block: ChatMessageBlock;
  hydratedPartIndex?: number;
  streaming: boolean;
  chatId?: string;
  cwd?: string;
  onAnswerQuestion?: (text: string) => boolean;
  onRespondInteraction?: ChatInteractionResponder;
  onRewind?: (t: number, text: string) => void;
  streamingPresentation: "blocks" | "tokens";
  actions?: AssistantTurnActions | null;
  onRetry?: (t: number, text: string) => void;
  onBranch?: (beforeT?: number) => void;
}) {
  if (block.type === "user") {
    return (
      <UserMessage
        text={block.text}
        t={block.t}
        userEmail={block.userEmail}
        chatId={chatId}
        cwd={cwd}
        onRewind={onRewind}
      />
    );
  }

  if (block.type === "error") {
    return <ErrorMessage message={block.message} t={block.t} />;
  }

  return (
    <AssistantMessage
      block={block}
      hydratedPartIndex={hydratedPartIndex}
      streaming={streaming}
      streamingPresentation={streamingPresentation}
      chatId={chatId}
      cwd={cwd}
      onAnswerQuestion={onAnswerQuestion}
      onRespondInteraction={onRespondInteraction}
      actions={actions}
      onRetry={onRetry}
      onBranch={onBranch}
    />
  );
}
