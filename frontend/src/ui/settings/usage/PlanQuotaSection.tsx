import { PLAN_QUOTA_TONES } from "../../../config/planQuota";
import type { PlanQuotaAccount, PlanQuotaWindow } from "../../../models/planQuota";
import { usePlanQuota } from "../../../state/hooks/usage/usePlanQuota";
import { Key } from "../../primitives/icons";

/**
 * Plan limits: how much of each Claude and Codex subscription account is left.
 *
 * It sits above the ledger because the two answer different questions with
 * different money. The ledger below is what this platform spent and can prove.
 * This is a rolling subscription window the vendor owns, spent from everywhere
 * the account is used — the operator's laptop included — which is why it can
 * move while the ledger does not.
 *
 * A plan belongs to one provider account, so a provider with several saved
 * accounts lists each account under its own label. Provider integrations
 * report normalized observations, so every window is a last-seen snapshot
 * with an age on it. Printing a stale figure as though it were current is how
 * an operator ends up planning a day's work against yesterday's number.
 */
export function PlanQuotaSection() {
  const { providers, loading } = usePlanQuota();
  if (loading || providers.length === 0) return null;

  return (
    <section class="rounded-card border border-line bg-surface px-4 py-3 space-y-3">
      <div class="flex items-center gap-2">
        <Key class="w-4 h-4 text-ink-300" />
        <h3 class="text-[13px] font-medium text-ink-100">Plan limits</h3>
        <span class="text-[11.5px] text-ink-400">
          what each subscription account reports during a run
        </span>
      </div>
      <ul class="divide-y divide-line">
        {providers.map((provider) => (
          <li key={provider.provider} class="py-2.5 first:pt-0 last:pb-0">
            <span class="text-[13px] font-medium text-ink-100">
              {provider.label}
            </span>
            {provider.accounts.map((account) => (
              <AccountPlan key={account.id} account={account} />
            ))}
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * One account's windows. When the current login is the provider's only plan,
 * the provider heading already names it.
 */
function AccountPlan({ account }: { account: PlanQuotaAccount }) {
  return (
    <div class="mt-2 first:mt-1.5">
      {account.label && (
        <div class="flex flex-wrap items-center gap-x-2 gap-y-0.5">
          <span class="truncate text-[12px] font-medium text-ink-200">
            {account.label}
          </span>
          {account.active && (
            <span class="rounded-full bg-accent-green/10 px-2 py-0.5 text-[10px] font-medium text-accent-green">
              Active
            </span>
          )}
          {account.detail && (
            <span class="truncate text-[11px] text-ink-400">{account.detail}</span>
          )}
        </div>
      )}
      {account.windows.map((window) => (
        <WindowRow key={window.kind} window={window} />
      ))}
    </div>
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
  const tone = PLAN_QUOTA_TONES[window.tone];
  return (
    <div class="mt-1.5">
      <div class="flex items-baseline justify-between gap-2">
        <span class="text-[11.5px] text-ink-300">{window.label}</span>
        <span class={`text-[11.5px] tabular-nums ${tone.textClass}`}>
          {window.percent == null
            ? tone.label
            : `${window.percent}% used`}
        </span>
      </div>
      {window.barPercent != null && (
        <div class="mt-1 h-1.5 overflow-hidden rounded-full bg-tint">
          <div
            class={`h-full rounded-full bg-current ${tone.textClass}`}
            style={{ width: `${window.barPercent}%` }}
          />
        </div>
      )}
      <p class="mt-0.5 text-[11px] text-ink-400">measured {window.measured}</p>
      {window.reset && (
        <p class="mt-0.5 text-[11px] text-ink-400">{window.reset}</p>
      )}
    </div>
  );
}
