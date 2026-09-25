import type { PlanQuotaPresentation, QuotaTone } from "../models/planQuota";

/** Visual urgency thresholds for reported subscription-plan usage. */
export const PLAN_QUOTA_WARNING_PERCENT = 70;
export const PLAN_QUOTA_SPENT_PERCENT = 90;

/** Refresh while the Usage tab is mounted, with at most one request in flight.
 *  The server asks the providers at most once a minute and may take up to
 *  half a minute to do so, so a request is given longer than that. */
export const PLAN_QUOTA_POLL_INTERVAL_MS = 15_000;
export const PLAN_QUOTA_REQUEST_TIMEOUT_MS = 45_000;
/** Ages and reset countdowns continue advancing even if a refresh fails. */
export const PLAN_QUOTA_CLOCK_INTERVAL_MS = 15_000;
/** A reading older than this says how old it is. */
export const PLAN_QUOTA_STALE_AFTER_MS = 5 * 60_000;

/** Each provider's windows as its own CLI shows them: Claude Code's /usage
 *  and Codex's /status. */
export const PLAN_QUOTA_PRESENTATIONS: Readonly<Record<string, PlanQuotaPresentation>> = {
  claude: {
    windows: { session: "Current session", weekly: "Current week (all models)" },
    measure: "used",
  },
  codex: {
    windows: { session: "5h limit", weekly: "Weekly limit" },
    measure: "left",
  },
};

export const PLAN_QUOTA_DEFAULT_PRESENTATION: PlanQuotaPresentation = {
  windows: { session: "5-hour limit", weekly: "Weekly limit" },
  measure: "used",
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
