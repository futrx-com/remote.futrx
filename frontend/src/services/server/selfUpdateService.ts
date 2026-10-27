import { SELF_UPDATE_CHECK_INTERVAL_MS } from "../../config/server.ts";
import type { SelfUpdateStatus } from "../../models/selfUpdate";

class SelfUpdateService {
  availableTag(status: SelfUpdateStatus | null): string | null {
    const check = status?.lastCheck;
    if (status?.run?.state === "running" || check?.error || !check?.updateAvailable) return null;
    return check.latestTag || null;
  }

  checkDue(lastAttempt: number | null, now: number): boolean {
    return lastAttempt === null || now - lastAttempt >= SELF_UPDATE_CHECK_INTERVAL_MS;
  }
}

export const selfUpdateService = new SelfUpdateService();
