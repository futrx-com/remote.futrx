import type { QuotaWindowKind } from "./agentQuota";

export type QuotaTone = "ok" | "warn" | "spent" | "unknown";

/** Render-ready projection consumed by the plan-limits section. */
export interface PlanQuotaWindow {
  kind: QuotaWindowKind;
  label: string;
  tone: QuotaTone;
  percent: number | null;
  barPercent: number | null;
  measured: string;
  reset: string;
}

/** One provider account's plan, as the section lists it. */
export interface PlanQuotaAccount {
  /** The saved account ID; empty for the provider's current login. */
  id: string;
  /** The saved account's label, or the current login's name beside saved
   *  accounts; empty when the current login is the provider's only plan. */
  label: string;
  /** What the account validated as, such as its email and plan type. */
  detail: string;
  /** The active account is the default for chats without a pinned account. */
  active: boolean;
  windows: PlanQuotaWindow[];
}

/** One provider and those of its accounts that have reported a window. */
export interface PlanQuotaProvider {
  provider: string;
  label: string;
  accounts: PlanQuotaAccount[];
}
