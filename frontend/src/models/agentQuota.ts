/**
 * Subscription quota: how much of a Claude or ChatGPT plan is left.
 *
 * This is not the usage dashboard. That counts what this platform spent; a
 * plan is spent from everywhere the operator works, so only the vendor knows
 * the total — and the agent CLIs are the vendor talking. They mention their
 * rolling windows in the middle of a run and offer no way to ask, so every
 * reading is a snapshot from the last run and carries when it was taken.
 */
export type QuotaWindowKind = "session" | "weekly";

export interface QuotaWindow {
  window: QuotaWindowKind;
  /** 0–100, absent when the CLI reports a status instead of a number. */
  usedPercent?: number;
  /** Unix seconds. 0 when the CLI did not say. */
  resetsAt?: number;
  /** The CLI's own word: "allowed", "allowed_warning", "rejected". */
  status?: string;
  /** Unix ms — when this platform saw it, not when it was true. */
  measuredAt: number;
}

export interface AgentQuota {
  provider: string;
  session?: QuotaWindow;
  weekly?: QuotaWindow;
}

export interface AgentQuotaResponse {
  agents?: AgentQuota[];
}

export type QuotaTone = "ok" | "warn" | "spent" | "unknown";

/** Render-ready projection consumed by the plan-limits section. */
export interface PlanQuotaWindow {
  kind: QuotaWindowKind;
  label: string;
  tone: QuotaTone;
  percent: number | null;
  barPercent: number | null;
  reset: string;
}

export interface PlanQuotaRow {
  provider: string;
  label: string;
  measured: string;
  windows: PlanQuotaWindow[];
}
