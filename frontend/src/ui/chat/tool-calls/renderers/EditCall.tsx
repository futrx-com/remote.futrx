import { useMemo } from "preact/hooks";
import { diffService } from "../../../../services/platform/diffService.ts";
import { Edit as EditIcon } from "../../../primitives/icons";
import type { ToolCallProps } from "../ToolCallTypes";
import { ToolShell } from "../ToolShell";
import { shortPath } from "../utils";

export function EditCall({ input, output, status, isError }: Omit<ToolCallProps, "name">) {
  const path = (input?.file_path as string) ?? "";
  const oldStr = (input?.old_string as string) ?? "";
  const newStr = (input?.new_string as string) ?? "";
  const edits = (input?.edits as Array<{ old_string: string; new_string: string }>) ?? null;

  const patches = useMemo(() => {
    if (edits && Array.isArray(edits)) {
      return edits.map((edit) => diffService.lines(edit.old_string ?? "", edit.new_string ?? ""));
    }
    return [diffService.lines(oldStr, newStr)];
  }, [oldStr, newStr, edits]);

  return (
    <ToolShell
      icon={<EditIcon class="w-4 h-4" />}
      label={<><span class="text-ink-300">Edit</span> <span class="font-mono">{shortPath(path)}</span></>}
      badge={edits ? `${edits.length} edits` : undefined}
      status={status}
      isError={isError}
      defaultOpen
    >
      <div class="divide-y divide-ink-500">
        {patches.map((parts, index) => (
          <pre key={index} class="overflow-x-auto touch-scroll p-3 text-[12.5px] leading-relaxed font-mono">
            {parts.map((part, partIndex) => (
              <span
                key={partIndex}
                class={
                  part.added ? "block bg-accent-green/15 text-accent-green" :
                  part.removed ? "block bg-accent-red/15 text-accent-red line-through" :
                  "block text-ink-200"
                }
              >
                {(part.added ? "+ " : part.removed ? "- " : "  ") + part.value.replace(/\n$/, "")}
              </span>
            ))}
          </pre>
        ))}
      </div>
      {output && isError ? <div class="p-3 text-accent-red font-mono text-xs">{output}</div> : null}
    </ToolShell>
  );
}
