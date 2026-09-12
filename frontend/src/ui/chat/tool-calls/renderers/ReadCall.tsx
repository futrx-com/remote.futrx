import { File } from "../../../primitives/icons";
import type { ToolCallProps } from "../ToolCallTypes";
import { CodeBlock } from "../CodeBlock";
import { ToolShell } from "../ToolShell";
import { READ_TOOL_OUTPUT_PREVIEW_CHARS } from "../../../../config/chat";
import { shortPath, truncate } from "../utils";

export function ReadCall({ input, output, outputExpanded, status, isError }: Omit<ToolCallProps, "name">) {
  const path = (input?.file_path as string) ?? "";
  return (
    <ToolShell
      icon={<File class="w-4 h-4" />}
      label={<><span class="text-ink-300">Read</span> <span class="font-mono">{shortPath(path)}</span></>}
      status={status}
      isError={isError}
    >
      {output ? <CodeBlock text={outputExpanded ? output : truncate(output, READ_TOOL_OUTPUT_PREVIEW_CHARS)} /> : null}
    </ToolShell>
  );
}
