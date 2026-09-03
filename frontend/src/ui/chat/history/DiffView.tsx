import { useMemo, useState } from "preact/hooks";
import { ChevronRight } from "../../primitives/icons";
import {
  displayPath,
  isDeletedFile,
  isNewFile,
  parseUnifiedDiff,
  type DiffFile,
  type DiffLine,
} from "./diffModel";

// Structured commit diff: one collapsible card per file with +/- counts, hunk
// separators, dual line-number gutters, and colored add/remove rows.
export function DiffView({ diff }: { diff: string }) {
  const files = useMemo(() => parseUnifiedDiff(diff), [diff]);

  if (files.length === 0) {
    return (
      <pre class="min-h-full w-max min-w-full p-4 text-[12px] leading-5 text-ink-100 font-mono whitespace-pre">
        {diff}
      </pre>
    );
  }

  return (
    <div class="p-3 space-y-3">
      {files.map((file) => (
        <FileCard key={`${file.oldPath}→${file.newPath}`} file={file} />
      ))}
    </div>
  );
}

function FileCard({ file }: { file: DiffFile }) {
  const [collapsed, setCollapsed] = useState(false);

  return (
    <section class="rounded-lg border border-line bg-inset overflow-hidden">
      <button
        type="button"
        onClick={() => setCollapsed((value) => !value)}
        class="w-full flex items-center gap-2 px-3 py-2 bg-tint hover:bg-tint text-left"
        title={collapsed ? "Expand file diff" : "Collapse file diff"}
      >
        <ChevronRight
          class={`w-3.5 h-3.5 flex-none text-ink-400 transition-transform ${collapsed ? "" : "rotate-90"}`}
        />
        <span class="min-w-0 flex-1 truncate font-mono text-[12.5px] text-ink-100">
          {displayPath(file)}
        </span>
        {isNewFile(file) && <FileBadge label="new" tone="text-accent-green border-accent-green/40 bg-accent-green/10" />}
        {isDeletedFile(file) && <FileBadge label="deleted" tone="text-accent-red border-accent-red/40 bg-accent-red/10" />}
        {file.binary && <FileBadge label="binary" tone="text-ink-300 border-line-strong bg-tint" />}
        {file.additions > 0 && (
          <span class="flex-none text-[11.5px] font-mono text-accent-green">+{file.additions}</span>
        )}
        {file.deletions > 0 && (
          <span class="flex-none text-[11.5px] font-mono text-accent-red">−{file.deletions}</span>
        )}
      </button>

      {!collapsed && (
        <div class="overflow-x-auto touch-scroll">
          {file.binary ? (
            <div class="px-3 py-2 text-[12px] text-ink-400">Binary file — no text diff.</div>
          ) : file.hunks.length === 0 ? (
            <div class="px-3 py-2 text-[12px] text-ink-400">No content changes (mode or rename only).</div>
          ) : (
            <table class="w-full border-collapse font-mono text-[12px] leading-5">
              <tbody>
                {file.hunks.map((hunk, hunkIndex) => (
                  <>
                    <tr key={`h-${hunkIndex}`}>
                      <td colSpan={3} class="px-3 py-1 bg-accent-blue/[0.08] text-accent-blue/90 text-[11.5px] whitespace-pre border-y border-line">
                        {hunk.header}
                      </td>
                    </tr>
                    {hunk.lines.map((line, lineIndex) => (
                      <DiffRow key={`l-${hunkIndex}-${lineIndex}`} line={line} />
                    ))}
                  </>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </section>
  );
}

function DiffRow({ line }: { line: DiffLine }) {
  const tone =
    line.kind === "add"
      ? "bg-accent-green/[0.10] text-ink-100"
      : line.kind === "del"
        ? "bg-accent-red/[0.10] text-ink-100"
        : line.kind === "meta"
          ? "text-ink-500"
          : "text-ink-200";
  const marker = line.kind === "add" ? "+" : line.kind === "del" ? "−" : " ";

  return (
    <tr class={tone}>
      <td class="w-11 min-w-11 px-2 text-right text-[11px] text-ink-500 select-none border-r border-line tabular-nums">
        {line.oldNo ?? ""}
      </td>
      <td class="w-11 min-w-11 px-2 text-right text-[11px] text-ink-500 select-none border-r border-line tabular-nums">
        {line.newNo ?? ""}
      </td>
      <td class="px-3 whitespace-pre">
        <span class="select-none inline-block w-3 text-ink-400">{marker}</span>
        {line.text}
      </td>
    </tr>
  );
}

function FileBadge({ label, tone }: { label: string; tone: string }) {
  return (
    <span class={`flex-none rounded border px-1.5 py-0.5 text-[10px] uppercase tracking-wide ${tone}`}>
      {label}
    </span>
  );
}
