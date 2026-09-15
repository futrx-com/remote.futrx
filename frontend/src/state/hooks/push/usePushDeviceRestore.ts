import { useEffect } from "preact/hooks";

import { pushSubscriptionApi } from "../../../api/pushSubscriptionApi";

// How long a tab may stay open before its registration is worth re-checking.
// Push services retire endpoints on their own schedule, and a workspace left
// open for days would otherwise only find out when a notification never came.
const REVALIDATE_AFTER_MS = 30 * 60 * 1000;

/**
 * Restores what this account already opted into on this device, on startup and
 * whenever a long-idle tab comes back to the foreground. A backend restart, a
 * deploy, or a push service retiring an endpoint can leave the browser with
 * nothing registered, and none of those are the user withdrawing permission.
 *
 * @param account the signed-in account, or "" while there is no session.
 */
export function usePushDeviceRestore(account: string): void {
  useEffect(() => {
    if (!account) return;
    let cancelled = false;
    let checkedAt = 0;

    const restore = () => {
      checkedAt = Date.now();
      void pushSubscriptionApi.ensureRegistered(account).catch(() => undefined);
    };
    const restoreIfStale = () => {
      if (cancelled || document.visibilityState !== "visible") return;
      if (Date.now() - checkedAt < REVALIDATE_AFTER_MS) return;
      restore();
    };

    restore();
    document.addEventListener("visibilitychange", restoreIfStale);
    return () => {
      cancelled = true;
      document.removeEventListener("visibilitychange", restoreIfStale);
    };
  }, [account]);
}
