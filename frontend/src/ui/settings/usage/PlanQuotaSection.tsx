import { PLAN_QUOTA_TONES } from "../../../config/planQuota";
import type { PlanQuotaAccount, PlanQuotaWindow } from "../../../models/planQuota";
import { usePlanQuota } from "../../../state/hooks/usage/usePlanQuota";
import { Key, Loader } from "../../primitives/icons";

/**
 * Plan limits: how much of each supported provider account's plan is left.
 *
 * It sits above the ledger because the two answer different questions. The
 * ledger below counts the tokens this platform spent and can prove.
 * This is a rolling subscription window the vendor owns, spent from everywhere
 * the account is used — the operator's laptop included — which is why it can
 * move while the ledger does not.
 *
 * The server asks each provider for its accounts' current limits, using the
 * provider's CLI or plan API. A plan belongs to one provider account, so one with
 * several saved accounts lists each account under its own label.
 */
export function PlanQuotaSection() {
  const { providers, loading } = usePlanQuota();
  if (!loading && providers.length === 0) return null;

  return (
    <section class="rounded-card border border-line bg-surface px-4 py-3 space-y-3">
      <div class="flex items-center gap-2">
        <Key class="w-4 h-4 text-ink-300" />
        <h3 class="text-[13px] font-medium text-ink-100">Plan limits</h3>
        <span class="text-[11.5px] text-ink-400">
          current usage of each subscription account
        </span>
      </div>
      {loading && providers.length === 0 ? (
        <div class="flex items-center gap-2 text-[12px] text-ink-400">
          <Loader class="w-3.5 h-3.5 animate-spin" /> Reading plan limits…
        </div>
      ) : (
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
      )}
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
      {account.error && (
        <p class="mt-1 text-[11px] text-accent-red">
          Couldn't read current limits: {account.error}
        </p>
      )}
      {account.windows.map((window) => (
        <WindowRow key={window.kind} window={window} />
      ))}
    </div>
  );
}

/**
 * One window, in the words of the provider's own CLI.
 *
 * A window with a percentage gets a bar. One with only a status gets the word
 * and nothing else — a status-only reading drawn at zero would read as "none
 * of your plan is used", which is a claim the provider never made.
 */
function WindowRow({ window }: { window: PlanQuotaWindow }) {
  const tone = PLAN_QUOTA_TONES[window.tone];
  return (
    <div class="mt-1.5">
      <div class="flex items-baseline justify-between gap-2">
        <span class="text-[11.5px] text-ink-300">{window.label}</span>
        <span class={`text-[11.5px] tabular-nums ${tone.textClass}`}>
          {window.value}
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
      {(window.reset || window.updated) && (
        <p class="mt-0.5 text-[11px] text-ink-400">
          {window.reset && <span title={window.resetIn || undefined}>{window.reset}</span>}
          {window.reset && window.updated && " · "}
          {window.updated}
        </p>
      )}
    </div>
  );
}
