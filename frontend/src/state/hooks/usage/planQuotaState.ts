import { PROVIDER_DISPLAY_LABELS } from "../../../config/agents.ts";
import {
  PLAN_QUOTA_MIN_VISIBLE_BAR_PERCENT,
  PLAN_QUOTA_SPENT_PERCENT,
  PLAN_QUOTA_WARNING_PERCENT,
} from "../../../config/usage.ts";
import type {
  AgentQuota,
  PlanQuotaRow,
  PlanQuotaWindow,
  QuotaTone,
  QuotaWindow,
  QuotaWindowKind,
} from "../../../models/agentQuota.ts";

const WINDOW_LABELS: Record<QuotaWindowKind, string> = {
  session: "5-hour window",
  weekly: "This week",
};

/** Projects API quota snapshots into the exact rows rendered by the Usage tab. */
export function projectPlanQuotaRows(
  quotas: AgentQuota[],
  nowMs: number
): PlanQuotaRow[] {
  return quotas.flatMap((quota) => {
    const windows = [
      projectWindow("session", quota.session, nowMs),
      projectWindow("weekly", quota.weekly, nowMs),
    ].filter((window): window is PlanQuotaWindow => window !== null);
    if (windows.length === 0) return [];

    return [
      {
        provider: quota.provider,
        label: PROVIDER_DISPLAY_LABELS[quota.provider] ?? quota.provider,
        measured: measuredAgo(quota.session ?? quota.weekly, nowMs),
        windows,
      },
    ];
  });
}

function projectWindow(
  kind: QuotaWindowKind,
  window: QuotaWindow | undefined,
  nowMs: number
): PlanQuotaWindow | null {
  if (!window) return null;
  const percent = typeof window.usedPercent === "number" ? Math.round(window.usedPercent) : null;
  return {
    kind,
    label: WINDOW_LABELS[kind],
    tone: quotaTone(window),
    percent,
    barPercent:
      percent === null
        ? null
        : Math.min(100, Math.max(PLAN_QUOTA_MIN_VISIBLE_BAR_PERCENT, percent)),
    reset: resetsIn(window, nowMs),
  };
}

function quotaTone(window: QuotaWindow): QuotaTone {
  const status = (window.status ?? "").toLowerCase();
  if (status === "rejected" || status === "exhausted") return "spent";
  if (typeof window.usedPercent === "number") {
    if (window.usedPercent >= PLAN_QUOTA_SPENT_PERCENT) return "spent";
    if (window.usedPercent >= PLAN_QUOTA_WARNING_PERCENT) return "warn";
    return "ok";
  }
  if (status === "allowed_warning") return "warn";
  if (status === "allowed") return "ok";
  return "unknown";
}

function resetsIn(window: QuotaWindow, nowMs: number): string {
  if (!window.resetsAt) return "";
  const seconds = window.resetsAt - Math.floor(nowMs / 1000);
  if (seconds <= 0) return "resets any moment";
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours >= 24) {
    const days = Math.floor(hours / 24);
    return `resets in ${days}d ${hours % 24}h`;
  }
  if (hours > 0) return `resets in ${hours}h ${minutes}m`;
  return `resets in ${minutes}m`;
}

function measuredAgo(window: QuotaWindow | undefined, nowMs: number): string {
  if (!window?.measuredAt) return "";
  const minutes = Math.floor((nowMs - window.measuredAt) / 60000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}
