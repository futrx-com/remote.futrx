import { providerDisplayLabel } from "../../../config/chat.ts";
import {
  PLAN_QUOTA_CURRENT_LOGIN_LABEL,
  PLAN_QUOTA_SPENT_PERCENT,
  PLAN_QUOTA_WARNING_PERCENT,
  PLAN_QUOTA_WINDOW_LABELS,
} from "../../../config/planQuota.ts";
import type {
  AccountQuota,
  QuotaWindow,
  QuotaWindowKind,
} from "../../../models/agentQuota.ts";
import type {
  AgentAuthAccount,
  AgentAuthAccountsSnapshot,
  AgentAuthProvider,
} from "../../../models/auth.ts";
import type {
  PlanQuotaAccount,
  PlanQuotaProvider,
  PlanQuotaWindow,
  QuotaTone,
} from "../../../models/planQuota.ts";

/**
 * Projects stored readings onto the provider accounts they describe, in the
 * order the Usage tab renders them.
 *
 * The agent-auth catalog owns accounts: which saved accounts a provider has,
 * what they are called, and which one is active. Each saved account that has
 * reported a window is listed in the catalog's order. A chat without a pinned
 * account runs on the active saved account, or on the provider's current
 * login while none is active, so the current login's reading is listed only
 * then. A removed account's reading belongs to no one and is never shown.
 */
export function projectPlanQuota(
  quotas: readonly AccountQuota[],
  authProviders: readonly AgentAuthProvider[],
  nowMs: number,
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
    const accounts = projectProviderAccounts(reported, entry?.status.accounts, nowMs);
    if (accounts.length === 0) return [];
    return [{ provider, label: entry?.label || providerDisplayLabel(provider), accounts }];
  });
}

function projectProviderAccounts(
  reported: ReadonlyMap<string, AccountQuota>,
  snapshot: AgentAuthAccountsSnapshot | undefined,
  nowMs: number,
): PlanQuotaAccount[] {
  const saved = snapshot?.items ?? [];
  const accounts = saved.flatMap((account) =>
    projectAccount(reported.get(account.id), account, nowMs)
  );
  if (snapshot?.activeAccountId) return accounts;
  const currentLogin = projectAccount(reported.get(""), undefined, nowMs).map((plan) => ({
    ...plan,
    label: saved.length > 0 ? PLAN_QUOTA_CURRENT_LOGIN_LABEL : "",
  }));
  return [...currentLogin, ...accounts];
}

function projectAccount(
  quota: AccountQuota | undefined,
  account: AgentAuthAccount | undefined,
  nowMs: number,
): PlanQuotaAccount[] {
  if (!quota) return [];
  const windows = [
    projectWindow("session", quota.session, nowMs),
    projectWindow("weekly", quota.weekly, nowMs),
  ].filter((window): window is PlanQuotaWindow => window !== null);
  if (windows.length === 0) return [];
  return [
    {
      id: quota.accountId,
      label: account?.label ?? "",
      detail: [account?.email, account?.planType].filter(Boolean).join(" · "),
      active: account?.active ?? false,
      windows,
    },
  ];
}

function projectWindow(
  kind: QuotaWindowKind,
  window: QuotaWindow | undefined,
  nowMs: number,
): PlanQuotaWindow | null {
  if (!window) return null;
  const usedPercent = reportedPercent(window.usedPercent);
  const percent = usedPercent === null ? null : Math.round(usedPercent);
  return {
    kind,
    label: PLAN_QUOTA_WINDOW_LABELS[kind],
    tone: quotaTone(window, usedPercent),
    percent,
    barPercent: percent === null ? null : Math.min(100, percent),
    measured: measuredAgo(window, nowMs),
    reset: resetsIn(window, nowMs),
  };
}

function reportedPercent(value: number | undefined): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? value
    : null;
}

function quotaTone(window: QuotaWindow, usedPercent: number | null): QuotaTone {
  const status = typeof window.status === "string" ? window.status.toLowerCase() : "";
  if (status === "rejected" || status === "exhausted") return "spent";
  if (usedPercent !== null) {
    if (usedPercent >= PLAN_QUOTA_SPENT_PERCENT) return "spent";
    if (usedPercent >= PLAN_QUOTA_WARNING_PERCENT) return "warn";
    return "ok";
  }
  if (status === "allowed_warning") return "warn";
  if (status === "allowed") return "ok";
  return "unknown";
}

function resetsIn(window: QuotaWindow, nowMs: number): string {
  if (!Number.isFinite(window.resetsAt) || !window.resetsAt || window.resetsAt < 0) return "";
  const seconds = window.resetsAt - Math.floor(nowMs / 1000);
  if (seconds <= 0) return "reset passed; awaiting a new reading";
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours >= 24) {
    const days = Math.floor(hours / 24);
    return `resets in ${days}d ${hours % 24}h`;
  }
  if (hours > 0) return `resets in ${hours}h ${minutes}m`;
  return `resets in ${minutes}m`;
}

function measuredAgo(window: QuotaWindow, nowMs: number): string {
  if (!Number.isFinite(window.measuredAt) || window.measuredAt <= 0) return "at an unknown time";
  const minutes = Math.floor((nowMs - window.measuredAt) / 60000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}
