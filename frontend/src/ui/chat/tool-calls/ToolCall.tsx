import { AskUserQuestion } from "./ask-user-question/AskUserQuestion";
import type { AskInput, ToolCallProps } from "./ToolCallTypes";
import { useTranscriptContent } from "../../../state/hooks/chat/useTranscriptContent";
import { BashCall } from "./renderers/BashCall";
import { EditCall } from "./renderers/EditCall";
import { GenericCall } from "./renderers/GenericCall";
import { ReadCall } from "./renderers/ReadCall";
import { SearchCall } from "./renderers/SearchCall";
import { WriteCall } from "./renderers/WriteCall";
import { toolOutputPreviewLimit } from "./utils";

export function ToolCall(props: ToolCallProps) {
  const {
    toolUseId,
    chatId,
    name,
    input,
    output,
    outputRef,
    outputBytes,
    onAnswerQuestion,
  } = props;
  const response = useTranscriptContent({
    chatId,
    content: output,
    contentRef: outputRef,
    contentBytes: outputBytes,
    inlinePreviewLimit: toolOutputPreviewLimit(name),
  });

  if (name === "AskUserQuestion" && toolUseId && chatId && onAnswerQuestion) {
    return (
      <AskUserQuestion
        toolUseId={toolUseId}
        chatId={chatId}
        input={(input as unknown as AskInput) ?? { questions: [] }}
        onSubmit={onAnswerQuestion}
      />
    );
  }

  const rendererProps = {
    ...props,
    output: response.content,
    outputExpanded: response.expanded,
  };
  let rendered;
  switch (name) {
    case "Read":
      rendered = <ReadCall {...rendererProps} />;
      break;
    case "Edit":
    case "MultiEdit":
      rendered = <EditCall {...rendererProps} />;
      break;
    case "Write":
      rendered = <WriteCall {...rendererProps} />;
      break;
    case "Bash":
      rendered = <BashCall {...rendererProps} />;
      break;
    case "Glob":
    case "Grep":
      rendered = <SearchCall {...rendererProps} />;
      break;
    default:
      rendered = <GenericCall {...rendererProps} />;
  }

  return (
    <>
      {rendered}
      {response.canExpand && (
        <div class="-mt-1 mb-2 flex items-center gap-2 px-2 text-[11px]">
          <button
            type="button"
            disabled={response.disabled}
            onClick={() => void response.load()}
            class="text-accent-blue hover:underline disabled:opacity-50"
          >
            {response.label}
          </button>
          {response.error && <span class="text-accent-red">{response.error}</span>}
        </div>
      )}
    </>
  );
}
