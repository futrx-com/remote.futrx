import type { QuotaWindowKind } from "./agentQuota";

export type QuotaTone = "ok" | "warn" | "spent" | "unknown";

/** How one provider's own CLI names its windows and states their usage:
 *  Claude Code's /usage counts what is used, Codex's /status what is left. */
export interface PlanQuotaPresentation {
  windows: Record<QuotaWindowKind, string>;
  measure: "used" | "left";
}

/** Render-ready projection consumed by the plan-limits section. */
export interface PlanQuotaWindow {
  kind: QuotaWindowKind;
  label: string;
  tone: QuotaTone;
  /** The figure the provider's CLI would print, e.g. "24% used" or "76% left",
   *  or the reported status when there is no figure. */
  value: string;
  /** Bar fill in the same measure as value; null when there is no figure. */
  barPercent: number | null;
  /** When the window resets, as the CLIs print it; empty when unknown. */
  reset: string;
  /** How long until the reset, for a tooltip; empty when unknown or past. */
  resetIn: string;
  /** How old a stale reading is; empty while it is current. */
  updated: string;
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
  /** Why the latest read of this account failed; empty when it succeeded. */
  error: string;
  windows: PlanQuotaWindow[];
}

/** One provider and those of its accounts that have reported a window. */
export interface PlanQuotaProvider {
  provider: string;
  label: string;
  accounts: PlanQuotaAccount[];
}
