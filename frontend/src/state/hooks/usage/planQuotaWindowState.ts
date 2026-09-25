import {
  PLAN_QUOTA_SPENT_PERCENT,
  PLAN_QUOTA_STALE_AFTER_MS,
  PLAN_QUOTA_TONE_LABELS,
  PLAN_QUOTA_WARNING_PERCENT,
} from "../../../config/planQuota.ts";
import type { QuotaWindow, QuotaWindowKind } from "../../../models/agentQuota.ts";
import type {
  PlanQuotaPresentation,
  PlanQuotaWindow,
  QuotaTone,
} from "../../../models/planQuota.ts";

/** Where reset times are shown: the browser's own locale and time zone
 *  unless a caller, such as a test, pins them. */
export interface PlanQuotaClock {
  nowMs: number;
  locale?: string;
  timeZone?: string;
}

/** Projects one provider reading into the value, tone, and timestamps its own
 * CLI would show. Account attribution and ordering stay with planQuotaState. */
export function projectPlanQuotaWindow(
  kind: QuotaWindowKind,
  window: QuotaWindow | undefined,
  presentation: PlanQuotaPresentation,
  clock: PlanQuotaClock,
): PlanQuotaWindow | null {
  if (!window) return null;
  const usedPercent = reportedPercent(window.usedPercent);
  const tone = quotaTone(window, usedPercent);
  const shown = shownPercent(usedPercent, presentation);
  return {
    kind,
    label: presentation.windows[kind],
    tone,
    value: shown === null ? PLAN_QUOTA_TONE_LABELS[tone] : `${shown}% ${presentation.measure}`,
    barPercent: shown === null ? null : Math.min(100, shown),
    reset: resetsAt(window, clock),
    resetIn: resetsIn(window, clock.nowMs),
    updated: updatedAgo(window, clock.nowMs),
  };
}

function reportedPercent(value: number | undefined): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0
    ? value
    : null;
}

/** The figure each CLI prints: Claude Code floors what is used; Codex rounds
 *  what is left, never below none. */
function shownPercent(
  usedPercent: number | null,
  presentation: PlanQuotaPresentation,
): number | null {
  if (usedPercent === null) return null;
  if (presentation.measure === "left") {
    return Math.round(Math.min(100, Math.max(0, 100 - usedPercent)));
  }
  return Math.floor(usedPercent);
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

function validReset(window: QuotaWindow): number | null {
  return Number.isFinite(window.resetsAt) && window.resetsAt && window.resetsAt > 0
    ? window.resetsAt
    : null;
}

/** The reset time as the CLIs print it: the time alone when it falls today,
 *  otherwise the date as well. */
function resetsAt(window: QuotaWindow, clock: PlanQuotaClock): string {
  const resetsAtSeconds = validReset(window);
  if (resetsAtSeconds === null) return "";
  if (resetsAtSeconds <= Math.floor(clock.nowMs / 1000)) {
    return "reset passed; awaiting a new reading";
  }
  const reset = new Date(resetsAtSeconds * 1000);
  const time = { hour: "numeric", minute: "2-digit", timeZone: clock.timeZone } as const;
  const day = (date: Date) => date.toLocaleDateString("en-CA", { timeZone: clock.timeZone });
  if (day(reset) === day(new Date(clock.nowMs))) {
    return `resets ${reset.toLocaleTimeString(clock.locale, time)}`;
  }
  return `resets ${reset.toLocaleString(clock.locale, { ...time, month: "short", day: "numeric" })}`;
}

function resetsIn(window: QuotaWindow, nowMs: number): string {
  const resetsAtSeconds = validReset(window);
  if (resetsAtSeconds === null) return "";
  const seconds = resetsAtSeconds - Math.floor(nowMs / 1000);
  if (seconds <= 0) return "";
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (hours >= 24) {
    const days = Math.floor(hours / 24);
    return `in ${days}d ${hours % 24}h`;
  }
  if (hours > 0) return `in ${hours}h ${minutes}m`;
  return `in ${minutes}m`;
}

/** How old a reading is once it has gone stale; a current one says nothing. */
function updatedAgo(window: QuotaWindow, nowMs: number): string {
  if (!Number.isFinite(window.measuredAt) || window.measuredAt <= 0) {
    return "updated at an unknown time";
  }
  const ageMs = nowMs - window.measuredAt;
  if (ageMs < PLAN_QUOTA_STALE_AFTER_MS) return "";
  const minutes = Math.floor(ageMs / 60000);
  if (minutes < 60) return `updated ${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `updated ${hours}h ago`;
  return `updated ${Math.floor(hours / 24)}d ago`;
}
