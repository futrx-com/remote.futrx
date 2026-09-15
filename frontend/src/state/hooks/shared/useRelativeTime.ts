import { relativeTimeService } from "../../../services/platform/relativeTimeService.ts";

/** Format at render time; the caller's renders determine when the label refreshes. */
export function useRelativeTime(timestamp: number): string {
  return relativeTimeService.ago(timestamp);
}
