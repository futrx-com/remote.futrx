import type { QuotaWindowKind } from "../models/agentQuota";
import type { QuotaTone } from "../models/planQuota";

/** Visual urgency thresholds for reported subscription-plan usage. */
export const PLAN_QUOTA_WARNING_PERCENT = 70;
export const PLAN_QUOTA_SPENT_PERCENT = 90;

/** Refresh while the Usage tab is mounted, with at most one request in flight. */
export const PLAN_QUOTA_POLL_INTERVAL_MS = 15_000;
export const PLAN_QUOTA_REQUEST_TIMEOUT_MS = 10_000;
/** Ages and reset countdowns continue advancing even if a refresh fails. */
export const PLAN_QUOTA_CLOCK_INTERVAL_MS = 15_000;

export const PLAN_QUOTA_WINDOW_LABELS: Record<QuotaWindowKind, string> = {
  session: "5-hour window",
  weekly: "This week",
};

/** Names the provider's current login when it is listed beside saved accounts. */
export const PLAN_QUOTA_CURRENT_LOGIN_LABEL = "Current login";

/** The text and color for each projected quota state. */
export const PLAN_QUOTA_TONES: Record<QuotaTone, { textClass: string; label: string }> = {
  ok: { textClass: "text-accent-blue", label: "fine" },
  warn: { textClass: "text-accent-orange", label: "getting low" },
  spent: { textClass: "text-accent-red", label: "out" },
  unknown: { textClass: "text-ink-400", label: "not reported" },
};
