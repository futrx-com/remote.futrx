import type { AssistantMessagePart } from "../../../models/chatMessage";
import { useState } from "preact/hooks";
import { ChevronDown, ChevronRight } from "../../primitives/icons";
import { Markdown } from "../markdown/Markdown";
import { CodeBlock } from "../tool-calls/CodeBlock";

type CollaborationPart = Extract<AssistantMessagePart, { kind: "collaboration" }>;
type SubagentTool = {
  id: string;
  name: string;
  status: string;
  isError: boolean;
  input?: unknown;
  output?: string;
  startedAt?: number;
  completedAt?: number;
  durationMs?: number;
};

export function CollaborationCard({
  part,
  chatId,
  cwd,
}: {
  part: CollaborationPart;
  chatId?: string;
  cwd?: string;
}) {
  const states = isObject(part.data.agentsStates) ? part.data.agentsStates : {};
  const isSubagentThread = part.data.type === "subagentThread";
  const receivers = Array.isArray(part.data.receiverThreadIds)
    ? part.data.receiverThreadIds.filter((item): item is string => typeof item === "string")
    : [];
  const tools = subagentTools(part.data.tools);
  const toolCount = typeof part.data.toolCount === "number" ? part.data.toolCount : tools.length;
  const failedToolCount = typeof part.data.failedToolCount === "number"
    ? part.data.failedToolCount
    : tools.filter((tool) => tool.isError).length;
  const label = part.name || "Subagent orchestration";
  const status = part.status || "inProgress";
  const [expanded, setExpanded] = useState(() => !isTerminalStatus(status));
  return (
    <section class="my-2 overflow-hidden rounded-lg border border-line bg-surface">
      <header class="bg-tint">
        <button
          type="button"
          aria-expanded={expanded}
          onClick={() => setExpanded((current) => !current)}
          class="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-tint-strong"
        >
          {expanded ? (
            <ChevronDown class="h-3.5 w-3.5 flex-none text-ink-300" aria-hidden="true" />
          ) : (
            <ChevronRight class="h-3.5 w-3.5 flex-none text-ink-300" aria-hidden="true" />
          )}
          <div class="min-w-0 flex-1">
            <div class="text-[12px] font-semibold text-ink-100">{label}</div>
            {receivers.length > 0 && (
              <div class="mt-0.5 truncate font-mono text-[10px] text-ink-400" title={receivers.join(", ")}>
                {receivers.map(shortThreadID).join(", ")}
              </div>
            )}
          </div>
          <span class="flex-none rounded-full border border-line-strong px-2 py-0.5 text-[10px] text-ink-300">
            {statusLabel(status)}
          </span>
        </button>
      </header>
      {expanded && <div class="space-y-2 border-t border-line p-3">
        {typeof part.data.prompt === "string" && (
          <p class="text-[12px] leading-relaxed text-ink-300">{part.data.prompt}</p>
        )}
        {isSubagentThread && tools.length > 0 && (
          <SubagentTools
            tools={tools}
            toolCount={toolCount}
            failedToolCount={failedToolCount}
          />
        )}
        {Object.entries(states).length === 0 ? (
          <p class="text-[11px] text-ink-400">{emptyStateMessage(status)}</p>
        ) : Object.entries(states).map(([threadId, value]) => {
          const state = isObject(value) ? value : {};
          return (
            <div key={threadId} class="rounded-control border border-line bg-canvas px-2.5 py-2">
              <div class="flex items-center justify-between gap-2">
                <span class="truncate font-mono text-[10px] text-ink-300">{threadId}</span>
                <span class="text-[10px] text-ink-400">{typeof state.status === "string" ? state.status : "unknown"}</span>
              </div>
              {typeof state.message === "string" && state.message && (
                <div class="codex-prose mt-2 text-[12px] leading-relaxed text-ink-200">
                  <Markdown chatId={chatId} cwd={cwd}>{state.message}</Markdown>
                </div>
              )}
              {isSubagentThread && !(typeof state.message === "string" && state.message) && (
                <div class="mt-2 text-[11px] text-ink-400">
                  {status === "inProgress" || status === "idle" ? "Working…" : "No final report was provided."}
                </div>
              )}
            </div>
          );
        })}
      </div>}
    </section>
  );
}

function SubagentTools({
  tools,
  toolCount,
  failedToolCount,
}: {
  tools: SubagentTool[];
  toolCount: number;
  failedToolCount: number;
}) {
  const [expanded, setExpanded] = useState(false);
  const toolNames = unique(tools.map((tool) => tool.name));
  return (
    <div class="overflow-hidden rounded-control border border-line bg-canvas">
      <button
        type="button"
        aria-expanded={expanded}
        onClick={() => setExpanded((current) => !current)}
        class="flex w-full items-center gap-2 px-2.5 py-2 text-left hover:bg-tint"
      >
        {expanded ? (
          <ChevronDown class="h-3 w-3 flex-none text-ink-400" aria-hidden="true" />
        ) : (
          <ChevronRight class="h-3 w-3 flex-none text-ink-400" aria-hidden="true" />
        )}
        <span class="text-[11px] font-medium text-ink-300">
          {toolCount} {toolCount === 1 ? "tool" : "tools"} used
        </span>
        <span class="min-w-0 flex-1 truncate text-[10px] text-ink-400" title={toolNames.join(", ")}>
          {toolNames.join(", ")}
        </span>
        {failedToolCount > 0 && (
          <span class="flex-none text-[10px] text-accent-red">
            {failedToolCount} failed
          </span>
        )}
      </button>
      {expanded && (
        <ol class="divide-y divide-line border-t border-line bg-inset">
          {tools.map((tool, index) => (
            <SubagentToolDetails key={tool.id || `${tool.name}-${index}`} tool={tool} index={index} />
          ))}
        </ol>
      )}
    </div>
  );
}

function SubagentToolDetails({ tool, index }: { tool: SubagentTool; index: number }) {
  const [expanded, setExpanded] = useState(false);
  const hasDetails = tool.input !== undefined || tool.output !== undefined;
  const timing = toolTiming(tool);
  return (
    <li>
      <button
        type="button"
        disabled={!hasDetails}
        aria-expanded={hasDetails ? expanded : undefined}
        onClick={() => hasDetails && setExpanded((current) => !current)}
        class="flex w-full items-center gap-2 px-3 py-1.5 text-left enabled:hover:bg-tint disabled:cursor-default"
      >
        {hasDetails ? (
          expanded ? (
            <ChevronDown class="h-3 w-3 flex-none text-ink-400" aria-hidden="true" />
          ) : (
            <ChevronRight class="h-3 w-3 flex-none text-ink-400" aria-hidden="true" />
          )
        ) : (
          <span class="h-3 w-3 flex-none" />
        )}
        <span class="w-4 flex-none text-right font-mono text-[9px] text-ink-500">
          {index + 1}
        </span>
        <span class="min-w-0 flex-1 truncate font-mono text-[10px] text-ink-300" title={tool.id}>
          {tool.name}
        </span>
        {timing.label && (
          <span class="flex-none font-mono text-[9px] text-ink-500" title={timing.title}>
            {timing.label}
          </span>
        )}
        <span class={`flex-none text-[10px] ${tool.isError ? "text-accent-red" : "text-ink-400"}`}>
          {tool.isError ? "failed" : statusLabel(tool.status)}
        </span>
      </button>
      {expanded && hasDetails && (
        <div class="divide-y divide-line border-t border-line bg-canvas">
          {tool.input !== undefined && (
            <div>
              <div class="bg-tint px-3 py-1 text-[10px] font-medium text-ink-400">Input</div>
              <CodeBlock text={formatToolInput(tool.input)} lang="json" />
            </div>
          )}
          {tool.output !== undefined && (
            <div>
              <div class="bg-tint px-3 py-1 text-[10px] font-medium text-ink-400">Output</div>
              <CodeBlock text={tool.output} />
            </div>
          )}
        </div>
      )}
    </li>
  );
}

function statusLabel(status: string): string {
  return status === "turnEnded" ? "turn ended" : status;
}

function isTerminalStatus(status: string): boolean {
  return ["completed", "failed", "interrupted", "cancelled", "canceled", "turnEnded"].includes(status);
}

function emptyStateMessage(status: string): string {
  if (status === "inProgress") return "Waiting for subagent status…";
  if (status === "turnEnded") {
    return "The turn ended before the provider reported a final subagent status.";
  }
  return "No final subagent status was reported.";
}

function shortThreadID(threadID: string): string {
  return threadID.length > 16 ? `${threadID.slice(0, 8)}…${threadID.slice(-4)}` : threadID;
}

function unique(values: string[]): string[] {
  return [...new Set(values)];
}

function subagentTools(value: unknown): SubagentTool[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((item) => {
    if (!isObject(item)) return [];
    return [{
      id: typeof item.id === "string" ? item.id : "",
      name: typeof item.name === "string" && item.name ? item.name : "Tool",
      status: typeof item.status === "string" && item.status ? item.status : "unknown",
      isError: item.isError === true,
      input: item.input,
      output: typeof item.output === "string" ? item.output : undefined,
      startedAt: typeof item.startedAt === "number" ? item.startedAt : undefined,
      completedAt: typeof item.completedAt === "number" ? item.completedAt : undefined,
      durationMs: typeof item.durationMs === "number" ? item.durationMs : undefined,
    }];
  });
}

function formatToolInput(input: unknown): string {
  if (typeof input === "string") return input;
  return JSON.stringify(input, null, 2) ?? String(input);
}

function toolTiming(tool: SubagentTool): { label: string; title?: string } {
  const timestamps = [
    tool.startedAt === undefined ? "" : `Started ${new Date(tool.startedAt).toLocaleString()}`,
    tool.completedAt === undefined ? "" : `Completed ${new Date(tool.completedAt).toLocaleString()}`,
  ].filter(Boolean).join(" · ");
  if (tool.durationMs === undefined) return { label: "", title: timestamps || undefined };
  return { label: formatDuration(tool.durationMs), title: timestamps || undefined };
}

function formatDuration(durationMs: number): string {
  if (durationMs < 1_000) return `${durationMs}ms`;
  if (durationMs < 60_000) return `${(durationMs / 1_000).toFixed(durationMs < 10_000 ? 1 : 0)}s`;
  const minutes = Math.floor(durationMs / 60_000);
  const seconds = Math.round((durationMs % 60_000) / 1_000);
  return `${minutes}m ${seconds}s`;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
