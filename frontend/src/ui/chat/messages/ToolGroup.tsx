import type { AssistantMessagePart } from "../../../models/chatMessage";
import { TerminalIcon } from "../../primitives/icons";
import { ToolCall } from "../tool-calls/ToolCall";
import { ToolShell } from "../tool-calls/ToolShell";

type ToolPart = Extract<AssistantMessagePart, { kind: "tool" }>;

export function ToolGroup({
  parts,
  startIndex,
  chatId,
  onAnswerQuestion,
}: {
  parts: ToolPart[];
  startIndex: number;
  chatId?: string;
  onAnswerQuestion?: (text: string) => void;
}) {
  const status = parts.some((part) => part.status === "running") ? "running" : "done";
  const isError = parts.some((part) => part.isError);
  const count = parts.length;
  const label = `${count} ${count === 1 ? "tool" : "tools"} used`;

  return (
    <ToolShell
      icon={<TerminalIcon class="w-4 h-4" />}
      label={<span class="font-medium">{label}</span>}
      status={status}
      isError={isError}
    >
      <div class="codex-tool-group-list p-2">
        {parts.map((part, offset) => (
          <ToolCall
            key={part.id || `${startIndex}-${offset}`}
            toolUseId={part.id}
            chatId={chatId}
            name={part.name}
            input={part.input}
            output={part.output}
            outputRef={part.outputRef}
            outputBytes={part.outputBytes}
            outputTruncated={part.outputTruncated}
            isError={part.isError}
            status={part.status}
            onAnswerQuestion={onAnswerQuestion}
          />
        ))}
      </div>
    </ToolShell>
  );
}
