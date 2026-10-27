import { providerDisplayLabel } from "../../../config/chat.ts";
import {
  PLAN_QUOTA_CURRENT_LOGIN_LABEL,
  PLAN_QUOTA_DEFAULT_PRESENTATION,
  PLAN_QUOTA_PRESENTATIONS,
} from "../../../config/planQuota.ts";
import type { AccountQuota } from "../../../models/agentQuota.ts";
import type {
  AgentAuthAccount,
  AgentAuthAccountsSnapshot,
  AgentAuthProvider,
} from "../../../models/auth.ts";
import type {
  PlanQuotaAccount,
  PlanQuotaPresentation,
  PlanQuotaProvider,
  PlanQuotaWindow,
} from "../../../models/planQuota.ts";
import { projectPlanQuotaWindow, type PlanQuotaClock } from "./planQuotaWindowState.ts";

/**
 * Projects stored readings onto the provider accounts they describe, in the
 * order the Usage tab renders them and in each provider's own words.
 *
 * The agent-auth catalog owns accounts: which saved accounts a provider has,
 * what they are called, and which one is active. Each saved account that has
 * reported a window, or whose latest read failed, is listed in the catalog's
 * order. A chat without a pinned account runs on the active saved account, or
 * on the provider's current login while none is active, so the current
 * login's reading is listed only then. A removed account's reading belongs to
 * no one and is never shown.
 */
export function projectPlanQuota(
  quotas: readonly AccountQuota[],
  authProviders: readonly AgentAuthProvider[],
  clock: PlanQuotaClock,
): PlanQuotaProvider[] {
  const readings = new Map<string, Map<string, AccountQuota>>();
  for (const quota of quotas) {
    const accounts = readings.get(quota.provider) ?? new Map<string, AccountQuota>();
    accounts.set(quota.accountId, quota);
    readings.set(quota.provider, accounts);
  }
  const catalogOrder = authProviders.map((entry) => entry.provider);
  const uncataloged = [...readings.keys()]
    .filter((provider) => !catalogOrder.includes(provider))
    .sort();

  return [...catalogOrder, ...uncataloged].flatMap((provider) => {
    const reported = readings.get(provider);
    if (!reported) return [];
    const entry = authProviders.find((candidate) => candidate.provider === provider);
    const presentation = PLAN_QUOTA_PRESENTATIONS[provider] ?? PLAN_QUOTA_DEFAULT_PRESENTATION;
    const accounts = projectProviderAccounts(reported, entry?.status.accounts, presentation, clock);
    if (accounts.length === 0) return [];
    return [{ provider, label: entry?.label || providerDisplayLabel(provider), accounts }];
  });
}

function projectProviderAccounts(
  reported: ReadonlyMap<string, AccountQuota>,
  snapshot: AgentAuthAccountsSnapshot | undefined,
  presentation: PlanQuotaPresentation,
  clock: PlanQuotaClock,
): PlanQuotaAccount[] {
  const saved = snapshot?.items ?? [];
  const accounts = saved.flatMap((account) =>
    projectAccount(reported.get(account.id), account, presentation, clock)
  );
  if (snapshot?.activeAccountId) return accounts;
  const currentLogin = projectAccount(reported.get(""), undefined, presentation, clock).map((plan) => ({
    ...plan,
    label: saved.length > 0 ? PLAN_QUOTA_CURRENT_LOGIN_LABEL : "",
  }));
  return [...currentLogin, ...accounts];
}

function projectAccount(
  quota: AccountQuota | undefined,
  account: AgentAuthAccount | undefined,
  presentation: PlanQuotaPresentation,
  clock: PlanQuotaClock,
): PlanQuotaAccount[] {
  if (!quota) return [];
  const windows = [
    projectPlanQuotaWindow("session", quota.session, presentation, clock),
    projectPlanQuotaWindow("weekly", quota.weekly, presentation, clock),
  ].filter((window): window is PlanQuotaWindow => window !== null);
  const error = quota.error ?? "";
  if (windows.length === 0 && !error) return [];
  return [
    {
      id: quota.accountId,
      label: account?.label ?? "",
      detail: [account?.email, account?.planType].filter(Boolean).join(" · "),
      active: account?.active ?? false,
      error,
      windows,
    },
  ];
}
