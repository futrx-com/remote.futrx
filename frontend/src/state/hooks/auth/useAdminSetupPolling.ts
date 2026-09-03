import { useEffect } from "preact/hooks";
import { ADMIN_SETUP_POLL_INTERVAL_MS } from "../../../config/auth";

export function useAdminSetupPolling(
  refresh: () => Promise<void>,
  enabled: boolean,
) {
  useEffect(() => {
    if (!enabled) return;

    let cancelled = false;
    let timer: number | undefined;
    const poll = async () => {
      try {
        await refresh();
      } catch {
        // A transient request failure should not stop setup discovery.
      } finally {
        if (!cancelled) timer = window.setTimeout(poll, ADMIN_SETUP_POLL_INTERVAL_MS);
      }
    };

    timer = window.setTimeout(poll, ADMIN_SETUP_POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }, [enabled, refresh]);
}
