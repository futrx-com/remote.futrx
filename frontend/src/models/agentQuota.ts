/**
 * Subscription quota: how much of a Claude or ChatGPT plan is left.
 *
 * This is not the usage dashboard. That counts what this platform spent; a
 * plan is spent from everywhere the operator works, so only the vendor knows
 * the total. Provider integrations obtain that total through their native
 * protocols, so every reading is a last-seen snapshot and carries when it was
 * taken.
 *
 * A plan belongs to one provider account. Readings name the saved account a
 * run used by its ID; the agent-auth catalog owns that account's label.
 */
export type QuotaWindowKind = "session" | "weekly";

export interface QuotaWindow {
  window: QuotaWindowKind;
  /** Nonnegative percentage; may exceed 100 when a plan is over its limit. */
  usedPercent?: number;
  /** Unix seconds. Absent when the CLI did not say. */
  resetsAt?: number;
  /** The CLI's own word: "allowed", "allowed_warning", "rejected". */
  status?: string;
  /** Unix ms — when this platform saw it, not when it was true. 0 if unknown. */
  measuredAt: number;
}

/** Every window one provider account has reported. */
export interface AccountQuota {
  provider: string;
  /** The saved account the windows belong to; empty for the host login. */
  accountId: string;
  session?: QuotaWindow;
  weekly?: QuotaWindow;
  /** Why the provider could not be asked for this account's limits just now;
   *  any windows are the last ones it reported. */
  error?: string;
}
