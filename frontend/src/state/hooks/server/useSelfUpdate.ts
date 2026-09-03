import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { selfUpdateApi } from "../../../api/selfUpdateApi";
import { SELF_UPDATE_RUNNING_POLL_INTERVAL_MS } from "../../../config/server";
import type { SelfUpdateStatus } from "../../../models/selfUpdate";

export function useSelfUpdate(enabled: boolean) {
  const [status, setStatus] = useState<SelfUpdateStatus | null>(null);
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(false);
  const [applying, setApplying] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestInFlight = useRef(false);
  const autoChecked = useRef(false);

  const running = status?.run?.state === "running";

  const check = useCallback(async () => {
    if (requestInFlight.current) return;
    requestInFlight.current = true;
    setChecking(true);
    try {
      setStatus(await selfUpdateApi.check());
      setError(null);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      requestInFlight.current = false;
      setChecking(false);
    }
  }, []);

  const apply = useCallback(async (tag?: string) => {
    if (requestInFlight.current) return;
    requestInFlight.current = true;
    setApplying(true);
    try {
      setStatus(await selfUpdateApi.apply(tag));
      setError(null);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      requestInFlight.current = false;
      setApplying(false);
    }
  }, []);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    setLoading(true);
    void (async () => {
      try {
        const next = await selfUpdateApi.status();
        if (cancelled) return;
        setStatus(next);
        setError(null);
        if (!autoChecked.current && next.run?.state !== "running") {
          autoChecked.current = true;
          void check();
        }
      } catch (cause) {
        if (!cancelled) setError((cause as Error).message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [enabled, check]);

  // While an update runs, keep polling. The updater restarts the backend, so
  // failed polls are expected mid-run — surface them as "restarting" instead
  // of an error and keep going until the new backend answers.
  useEffect(() => {
    if (!enabled || !running) return;
    const interval = window.setInterval(() => {
      void (async () => {
        try {
          const next = await selfUpdateApi.status();
          setStatus(next);
          setRestarting(false);
        } catch {
          setRestarting(true);
        }
      })();
    }, SELF_UPDATE_RUNNING_POLL_INTERVAL_MS);
    return () => window.clearInterval(interval);
  }, [enabled, running]);

  return { status, loading, checking, applying, restarting, error, check, apply };
}
