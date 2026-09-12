import type { PlanQuotaWindow, QuotaTone } from "../../../models/agentQuota";
import { usePlanQuota } from "../../../state/hooks/usage/usePlanQuota";
import { Key } from "../../primitives/icons";

const TONE_TEXT: Record<QuotaTone, string> = {
  ok: "text-accent-blue",
  warn: "text-accent-orange",
  spent: "text-accent-red",
  unknown: "text-ink-400",
};

const TONE_WORD: Record<QuotaTone, string> = {
  ok: "fine",
  warn: "getting low",
  spent: "out",
  unknown: "not reported",
};

/**
 * Plan limits: how much of the Claude and Codex subscriptions is left.
 *
 * It sits above the ledger because the two answer different questions with
 * different money. The ledger below is what this platform spent and can prove.
 * This is a rolling subscription window the vendor owns, spent from everywhere
 * the operator works — their laptop included — which is why it can move while
 * the ledger does not.
 *
 * Provider integrations report normalized observations, so every row is a
 * last-seen snapshot with an age on it. Printing a stale figure as though it
 * were current is how an operator ends up planning a day's work against
 * yesterday's number.
 */
export function PlanQuotaSection() {
  const { rows, loading } = usePlanQuota();
  if (loading || rows.length === 0) return null;

  return (
    <section class="rounded-card border border-line bg-surface px-4 py-3 space-y-3">
      <div class="flex items-center gap-2">
        <Key class="w-4 h-4 text-ink-300" />
        <h3 class="text-[13px] font-medium text-ink-100">Plan limits</h3>
        <span class="text-[11.5px] text-ink-400">
          what your subscriptions report during a run
        </span>
      </div>
      <ul class="divide-y divide-line">
        {rows.map((row) => (
          <li key={row.provider} class="py-2.5 first:pt-0 last:pb-0">
            <div class="flex flex-wrap items-baseline gap-x-2">
              <span class="text-[13px] font-medium text-ink-100">
                {row.label}
              </span>
              <span class="text-[11px] text-ink-400">
                measured {row.measured}
              </span>
            </div>
            {row.windows.map((window) => (
              <WindowRow key={window.kind} window={window} />
            ))}
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * One window.
 *
 * A window with a percentage gets a bar. One with only a status gets the word
 * and nothing else — Claude usually reports "allowed" and no number, and a bar
 * drawn at zero would read as "none of your plan is used", which is a claim
 * the CLI never made.
 */
function WindowRow({ window }: { window: PlanQuotaWindow }) {
  return (
    <div class="mt-1.5">
      <div class="flex items-baseline justify-between gap-2">
        <span class="text-[11.5px] text-ink-300">{window.label}</span>
        <span class={`text-[11.5px] tabular-nums ${TONE_TEXT[window.tone]}`}>
          {window.percent == null
            ? TONE_WORD[window.tone]
            : `${window.percent}% used`}
        </span>
      </div>
      {window.barPercent != null && (
        <div class="mt-1 h-1.5 overflow-hidden rounded-full bg-tint">
          <div
            class={`h-full rounded-full bg-current ${TONE_TEXT[window.tone]}`}
            style={{ width: `${window.barPercent}%` }}
          />
        </div>
      )}
      {window.reset && (
        <p class="mt-0.5 text-[11px] text-ink-400">{window.reset}</p>
      )}
    </div>
  );
}
