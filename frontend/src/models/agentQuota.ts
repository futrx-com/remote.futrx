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

const AGENT_LABELS: Record<string, string> = {
  claude: "Claude",
  codex: "Codex",
  kimi: "Kimi",
  antigravity: "Antigravity",
};

export function agentQuotaLabel(provider: string): string {
  return AGENT_LABELS[provider] ?? provider;
}

export function windowLabel(kind: QuotaWindowKind): string {
  return kind === "session" ? "5-hour window" : "This week";
}

/**
 * How a window reads at a glance.
 *
 * "unknown" is a real state and the most common one on Claude, which sends a
 * status and no number. It renders as text rather than as a bar, because a bar
 * at zero would say the plan is untouched.
 */
export type QuotaTone = "ok" | "warn" | "spent" | "unknown";

export function quotaTone(win: QuotaWindow | undefined): QuotaTone {
  if (!win) return "unknown";
  const status = (win.status ?? "").toLowerCase();
  if (status === "rejected" || status === "exhausted") return "spent";
  if (typeof win.usedPercent === "number") {
    if (win.usedPercent >= 90) return "spent";
    if (win.usedPercent >= 70) return "warn";
    return "ok";
  }
  if (status === "allowed_warning") return "warn";
  if (status === "allowed") return "ok";
  return "unknown";
}

/** "resets in 2h 40m", or "" when the CLI gave no reset time. */
export function resetsIn(win: QuotaWindow | undefined, nowMs: number): string {
  if (!win?.resetsAt) return "";
  const seconds = win.resetsAt - Math.floor(nowMs / 1000);
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

/**
 * "measured 4 minutes ago" — the caveat that makes the number honest.
 *
 * Readings arrive only during a run, so an idle platform shows an old one. The
 * card says how old rather than implying it is live.
 */
export function measuredAgo(win: QuotaWindow | undefined, nowMs: number): string {
  if (!win?.measuredAt) return "";
  const minutes = Math.floor((nowMs - win.measuredAt) / 60000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}
