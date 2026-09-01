import { AskUserQuestion } from "./ask-user-question/AskUserQuestion";
import type { AskUserQuestionInput } from "../../../models/chat.ts";
import type { ToolCallProps } from "./ToolCallTypes";
import { BashCall } from "./renderers/BashCall";
import { EditCall } from "./renderers/EditCall";
import { GenericCall } from "./renderers/GenericCall";
import { ReadCall } from "./renderers/ReadCall";
import { SearchCall } from "./renderers/SearchCall";
import { WriteCall } from "./renderers/WriteCall";

export function ToolCall({ toolUseId, chatId, name, input, output, isError, status, interactive, interactionRequestedAt, onAnswerQuestion, onInteractionActivity }: ToolCallProps) {
  if (name === "AskUserQuestion" && toolUseId && chatId && onAnswerQuestion) {
    return (
      <AskUserQuestion
        toolUseId={toolUseId}
        chatId={chatId}
        input={(input as unknown as AskUserQuestionInput) ?? { questions: [] }}
        onSubmit={(answer) => onAnswerQuestion({
          ...answer,
          interactionId: interactive ? toolUseId : undefined,
        })}
        onActivity={interactive && onInteractionActivity
          ? () => onInteractionActivity(toolUseId)
          : undefined}
        interactive={interactive}
        requestedAt={interactionRequestedAt}
        resolved={interactive && status === "done"}
        resolvedOutput={output}
      />
    );
  }

  switch (name) {
    case "Read":
      return <ReadCall input={input} output={output} status={status} isError={isError} />;
    case "Edit":
    case "MultiEdit":
      return <EditCall input={input} output={output} status={status} isError={isError} />;
    case "Write":
      return <WriteCall input={input} output={output} status={status} isError={isError} />;
    case "Bash":
      return <BashCall input={input} output={output} status={status} isError={isError} />;
    case "Glob":
    case "Grep":
      return <SearchCall name={name} input={input} output={output} status={status} isError={isError} />;
    default:
      return <GenericCall name={name} input={input} output={output} status={status} isError={isError} />;
  }
}
