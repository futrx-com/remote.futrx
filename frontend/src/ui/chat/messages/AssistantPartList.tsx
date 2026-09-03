import type { ComponentChildren } from "preact";
import { providerDisplayLabel } from "../../../config/chat";
import type { AssistantMessagePart } from "../../../models/chatMessage";
import { ToolCall } from "../tool-calls/ToolCall";
import { StreamingText } from "./StreamingText";
import { ThinkingBlock } from "./ThinkingBlock";
import { ToolGroup } from "./ToolGroup";
import { InteractionCard } from "../interactions/InteractionCard";
import { CollaborationCard } from "./CollaborationCard";
import type { ChatInteractionResponder } from "../../../types/chatApi";

type ToolPart = Extract<AssistantMessagePart, { kind: "tool" }>;

export function AssistantPartList({
  parts,
  streaming,
  chatId,
  cwd,
  onAnswerQuestion,
  onRespondInteraction,
}: {
  parts: AssistantMessagePart[];
  streaming: boolean;
  chatId?: string;
  cwd?: string;
  onAnswerQuestion?: (text: string) => void;
  onRespondInteraction?: ChatInteractionResponder;
}) {
  return <>{renderAssistantParts(parts, { streaming, chatId, cwd, onAnswerQuestion, onRespondInteraction })}</>;
}

function renderAssistantParts(
  parts: AssistantMessagePart[],
  context: {
    streaming: boolean;
    chatId?: string;
    cwd?: string;
    onAnswerQuestion?: (text: string) => void;
    onRespondInteraction?: ChatInteractionResponder;
  }
): ComponentChildren[] {
  const rendered: ComponentChildren[] = [];
  let toolGroup: ToolPart[] = [];
  let toolGroupStart = 0;

  const flushToolGroup = () => {
    if (!toolGroup.length) return;
    rendered.push(
      <ToolGroup
        key={`tools-${toolGroupStart}`}
        parts={toolGroup}
        startIndex={toolGroupStart}
        chatId={context.chatId}
        onAnswerQuestion={context.onAnswerQuestion}
      />
    );
    toolGroup = [];
  };

  parts.forEach((part, index) => {
    if (part.kind === "tool" && isGroupableTool(part)) {
      if (!toolGroup.length) toolGroupStart = index;
      toolGroup.push(part);
      return;
    }

    flushToolGroup();

    if (part.kind === "text") {
      rendered.push(
        <div key={index} class="codex-prose min-w-0 max-w-full text-[14.5px] leading-[1.7] text-ink-100 [overflow-wrap:anywhere]">
          <StreamingText text={part.text} streaming={context.streaming} chatId={context.chatId} cwd={context.cwd} />
        </div>
      );
      return;
    }

    if (part.kind === "thinking") {
      rendered.push(
        <ThinkingBlock
          key={`thinking-${index}`}
          text={part.text}
          active={context.streaming && index === parts.length - 1}
        />
      );
      return;
    }

    if (part.kind === "interaction") {
      rendered.push(
        <InteractionCard key={part.id} part={part} onRespond={context.onRespondInteraction} />
      );
      return;
    }

    if (part.kind === "collaboration") {
      rendered.push(
        <CollaborationCard
          key={part.id}
          part={part}
          chatId={context.chatId}
          cwd={context.cwd}
        />
      );
      return;
    }

    if (part.kind === "turn-status") {
      const providerLabel = part.provider ? providerDisplayLabel(part.provider) : "Agent";
      rendered.push(
        <div key={`status-${index}`} class="my-2 flex items-center gap-2 text-[11px] text-ink-400">
          <span class="h-1.5 w-1.5 rounded-full bg-accent-blue" aria-hidden="true" />
          {providerLabel} turn: {part.status}
        </div>
      );
      return;
    }

    rendered.push(
      <ToolCall
        key={part.id || index}
        toolUseId={part.id}
        chatId={context.chatId}
        name={part.name}
        input={part.input}
        output={part.output}
        isError={part.isError}
        status={part.status}
        onAnswerQuestion={context.onAnswerQuestion}
      />
    );
  });

  flushToolGroup();
  return rendered;
}

function isGroupableTool(part: ToolPart): boolean {
  return part.name !== "AskUserQuestion";
}
