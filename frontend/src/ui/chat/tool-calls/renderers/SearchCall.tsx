import { TerminalIcon } from "../../../primitives/icons";
import type { ToolCallProps } from "../ToolCallTypes";
import { CodeBlock } from "../CodeBlock";
import { ToolShell } from "../ToolShell";
import { DEFAULT_TOOL_OUTPUT_PREVIEW_CHARS } from "../../../../config/chat";
import { shortPath, truncate } from "../utils";

export function SearchCall({ name, input, output, outputExpanded, status, isError }: ToolCallProps) {
  const pattern = (input?.pattern as string) ?? (input?.query as string) ?? "";
  const path = (input?.path as string) ?? "";
  return (
    <ToolShell
      icon={<TerminalIcon class="w-4 h-4" />}
      label={
        <>
          <span class="text-ink-300">{name}</span>{" "}
          <span class="font-mono">{pattern}</span>
          {path && <span class="text-ink-300 ml-1">in {shortPath(path)}</span>}
        </>
      }
      status={status}
      isError={isError}
      revealSignal={outputExpanded}
    >
      {output ? <CodeBlock text={outputExpanded ? output : truncate(output, DEFAULT_TOOL_OUTPUT_PREVIEW_CHARS)} /> : null}
    </ToolShell>
  );
}
