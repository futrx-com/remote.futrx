import { File } from "../../../primitives/icons";
import type { ToolCallProps } from "../ToolCallTypes";
import { CodeBlock } from "../CodeBlock";
import { ToolShell } from "../ToolShell";
import { shortPath, truncate } from "../utils";

export function WriteCall({ input, output, outputExpanded, status, isError }: Omit<ToolCallProps, "name">) {
  const path = (input?.file_path as string) ?? "";
  const content = (input?.content as string) ?? "";
  return (
    <ToolShell
      icon={<File class="w-4 h-4" />}
      label={<><span class="text-ink-300">Write</span> <span class="font-mono">{shortPath(path)}</span></>}
      badge={`${content.split("\n").length} lines`}
      status={status}
      isError={isError}
    >
      <CodeBlock text={truncate(content, 8000)} />
      {output && (isError || outputExpanded) ? (
        <div class={`border-t border-ink-500 p-3 font-mono text-xs ${isError ? "text-accent-red" : "text-ink-300"}`}>{output}</div>
      ) : null}
    </ToolShell>
  );
}
