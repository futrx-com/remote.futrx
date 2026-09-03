import type { ComponentChildren } from "preact";
import type { UsageGroup, UsageGroupBy, UsageRecord } from "../../../models/usage";
import { usageFormatService } from "../../../services/usage/usageFormatService.ts";
import { ChevronRight, Loader, X } from "../../primitives/icons";

const GROUP_HEADINGS: Record<UsageGroupBy, string> = {
  project: "Project",
  user: "User",
  provider: "Provider",
  model: "Model",
  day: "Day",
  chat: "Chat",
};

export function UsageGroupTable({
  groups,
  groupBy,
  onDrillDown,
}: {
  groups: UsageGroup[];
  groupBy: UsageGroupBy;
  onDrillDown: (group: UsageGroup) => void;
}) {
  if (groups.length === 0) {
    return (
      <UsagePanel title={GROUP_HEADINGS[groupBy]} description="No usage recorded in this range.">
        <div class="px-1 py-6 text-center text-[13px] text-ink-400">Nothing to show yet.</div>
      </UsagePanel>
    );
  }

  // Drilling down asks for the raw runs of one project, so the affordance is
  // only meaningful while the table is grouped by project.
  const drillable = groupBy === "project";
  return (
    <UsagePanel
      title={`Usage by ${GROUP_HEADINGS[groupBy].toLowerCase()}`}
      description={`${groups.length} row${groups.length === 1 ? "" : "s"}${
        drillable ? " · select a project to see its runs" : ""
      }`}
    >
      <UsageScroller>
        <table class="w-full text-[13px] border-collapse min-w-[560px]">
          <thead>
            <tr class="text-left text-[11.5px] uppercase tracking-wide text-ink-400">
              <th class="font-medium px-2 py-2">{GROUP_HEADINGS[groupBy]}</th>
              <th class="font-medium px-2 py-2 text-right">Runs</th>
              <th class="font-medium px-2 py-2 text-right">Tokens</th>
              <th class="font-medium px-2 py-2 text-right">Cost</th>
              {drillable && <th class="w-8" />}
            </tr>
          </thead>
          <tbody>
            {groups.map((group) => (
              <tr
                key={group.key || group.label}
                class={`border-t border-line ${
                  drillable ? "cursor-pointer hover:bg-tint" : ""
                }`}
                onClick={drillable ? () => onDrillDown(group) : undefined}
              >
                <td class="px-2 py-2 text-ink-100 truncate max-w-[220px]">{group.label}</td>
                <td class="px-2 py-2 text-right text-ink-200 font-mono">{group.runs}</td>
                <td class="px-2 py-2 text-right text-ink-200 font-mono">
                  {usageFormatService.tokens(group.totalTokens)}
                </td>
                <td class="px-2 py-2 text-right font-mono text-ink-50">
                  <CostCell
                    cost={group.costUsd}
                    estimated={group.estimatedCostUsd}
                    unpriced={group.unpricedRuns}
                  />
                </td>
                {drillable && (
                  <td class="px-1 py-2 text-ink-400">
                    <ChevronRight class="w-4 h-4" />
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </UsageScroller>
    </UsagePanel>
  );
}

export function UsageRecordsTable({
  label,
  records,
  loading,
  error,
  hasMore,
  onClose,
  onLoadMore,
}: {
  label: string;
  records: UsageRecord[];
  loading: boolean;
  error: string | null;
  hasMore: boolean;
  onClose: () => void;
  onLoadMore: () => void;
}) {
  return (
    <UsagePanel
      title={`Runs in ${label}`}
      description={
        error
          ? `Could not load runs: ${error}`
          : `${records.length} run${records.length === 1 ? "" : "s"}${hasMore ? " (more available)" : ""}`
      }
      action={
        <button
          type="button"
          onClick={onClose}
          class="h-8 w-8 rounded-md text-ink-300 hover:text-ink-50 hover:bg-tint-strong grid place-items-center"
          aria-label="Close run list"
        >
          <X class="w-4 h-4" />
        </button>
      }
    >
      {loading && records.length === 0 ? (
        <div class="px-1 py-6 flex items-center justify-center gap-2 text-[13px] text-ink-300">
          <Loader class="w-4 h-4 animate-spin" /> Loading runs…
        </div>
      ) : records.length === 0 ? (
        <div class="px-1 py-6 text-center text-[13px] text-ink-400">No runs in this range.</div>
      ) : (
        <>
          <UsageScroller>
            <table class="w-full text-[12.5px] border-collapse min-w-[720px]">
              <thead>
                <tr class="text-left text-[11.5px] uppercase tracking-wide text-ink-400">
                  <th class="font-medium px-2 py-2">When (UTC)</th>
                  <th class="font-medium px-2 py-2">Chat</th>
                  <th class="font-medium px-2 py-2">User</th>
                  <th class="font-medium px-2 py-2">Model</th>
                  <th class="font-medium px-2 py-2 text-right">In</th>
                  <th class="font-medium px-2 py-2 text-right">Out</th>
                  <th class="font-medium px-2 py-2 text-right">Cache</th>
                  <th class="font-medium px-2 py-2 text-right">Cost</th>
                </tr>
              </thead>
              <tbody>
                {records.map((record) => (
                  <tr
                    key={`${record.chatId}-${record.at}`}
                    class="border-t border-line align-top"
                  >
                    <td class="px-2 py-2 text-ink-300 font-mono whitespace-nowrap">
                      {formatUtcDateTime(record.at)}
                    </td>
                    <td class="px-2 py-2 text-ink-200 font-mono">
                      {record.chatId}
                      {record.scheduled && (
                        <span class="ml-1.5 text-[10.5px] text-accent-green">scheduled</span>
                      )}
                    </td>
                    <td class="px-2 py-2 text-ink-200 truncate max-w-[180px]">
                      {record.userEmail || "—"}
                    </td>
                    <td class="px-2 py-2 text-ink-200 truncate max-w-[180px]">
                      {record.model || record.provider || "—"}
                    </td>
                    <td class="px-2 py-2 text-right text-ink-200 font-mono">
                      {usageFormatService.tokens(record.inputTokens)}
                    </td>
                    <td class="px-2 py-2 text-right text-ink-200 font-mono">
                      {usageFormatService.tokens(record.outputTokens)}
                    </td>
                    <td class="px-2 py-2 text-right text-ink-300 font-mono">
                      {usageFormatService.tokens(record.cacheReadTokens + record.cacheWriteTokens)}
                    </td>
                    <td class="px-2 py-2 text-right font-mono text-ink-50">
                      {record.costUsd == null ? (
                        <span class="text-ink-400" title="No provider price and no matching price-table entry">
                          unknown
                        </span>
                      ) : (
                        <span title={record.estimated ? "Estimated from the price table" : "Reported by the provider"}>
                          {record.estimated ? "~" : ""}
                          {usageFormatService.usd(record.costUsd)}
                        </span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </UsageScroller>
          {hasMore && (
            <button
              type="button"
              onClick={onLoadMore}
              disabled={loading}
              class="mt-2 h-9 px-3 rounded-md bg-tint-strong hover:bg-tint-active text-[13px]
                     text-ink-100 disabled:opacity-60"
            >
              {loading ? "Loading…" : "Load more runs"}
            </button>
          )}
        </>
      )}
    </UsagePanel>
  );
}

function CostCell({
  cost,
  estimated,
  unpriced,
}: {
  cost: number;
  estimated: number;
  unpriced: number;
}) {
  const allEstimated = cost > 0 && estimated >= cost;
  const title = [
    estimated > 0 ? `${usageFormatService.usd(estimated)} estimated from the price table` : null,
    unpriced > 0 ? `${unpriced} run${unpriced === 1 ? "" : "s"} with unknown cost` : null,
  ]
    .filter(Boolean)
    .join(" · ");
  return (
    <span title={title || "Reported by the provider"}>
      {allEstimated ? "~" : ""}
      {usageFormatService.usd(cost)}
      {!allEstimated && estimated > 0 && <span class="text-ink-400">*</span>}
    </span>
  );
}

export function UsagePanel({
  title,
  description,
  action,
  children,
}: {
  title: string;
  description: string;
  action?: ComponentChildren;
  children: ComponentChildren;
}) {
  return (
    <section class="rounded-card border border-line bg-surface overflow-hidden">
      <header class="px-4 py-3 border-b border-line flex items-start gap-3">
        <div class="flex-1 min-w-0">
          <div class="text-[14.5px] font-semibold text-ink-50">{title}</div>
          <div class="text-[12.5px] text-ink-300 mt-0.5 leading-snug">{description}</div>
        </div>
        {action}
      </header>
      <div class="p-3">{children}</div>
    </section>
  );
}

function UsageScroller({ children }: { children: ComponentChildren }) {
  return <div class="overflow-x-auto touch-scroll">{children}</div>;
}

/** Records are bucketed by UTC day server-side, so they are shown in UTC. */
function formatUtcDateTime(at: number): string {
  return new Date(at).toISOString().replace("T", " ").slice(0, 16);
}
