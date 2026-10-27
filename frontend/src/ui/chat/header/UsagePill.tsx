import type { ChatUsageTotals } from "../../../models/chatUsage";
import { Activity } from "../../primitives/icons";

export function UsagePill({
  totals,
  tokenLabel,
}: {
  totals: ChatUsageTotals;
  tokenLabel: string;
}) {
  return (
    <div
      class="h-9 inline-flex items-center gap-2 px-3 rounded-md bg-tint border border-line
             text-[12.5px] text-ink-300 flex-none"
      title={`Input ${totals.inputTokens}\nOutput ${totals.outputTokens}\nCache read ${totals.cacheReadTokens}\nCache write ${totals.cacheWriteTokens}`}
    >
      <Activity class="w-4 h-4 text-accent-green" />
      <span>{tokenLabel} tokens</span>
    </div>
  );
}
