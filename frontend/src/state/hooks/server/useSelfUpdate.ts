import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { selfUpdateApi } from "../../../api/selfUpdateApi";
import { SELF_UPDATE_CHECK_INTERVAL_MS, SELF_UPDATE_RUNNING_POLL_INTERVAL_MS } from "../../../config/server";
import type { SelfUpdateStatus } from "../../../models/selfUpdate";
import { selfUpdateService } from "../../../services/server/selfUpdateService";
import { frontendBuildStore } from "../../stores/server/frontendBuildStore.ts";

export function useSelfUpdate(enabled: boolean) {
  const [status, setStatus] = useState<SelfUpdateStatus | null>(null);
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(false);
  const [applying, setApplying] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestInFlight = useRef(false);
  const lastAttempt = useRef<number | null>(null);
  const session = useRef(0);

  // Invalidate requests when the admin workspace closes or access changes.
  useEffect(() => {
    setStatus(null);
    setError(null);
    setLoading(false);
    setChecking(false);
    setApplying(false);
    setRestarting(false);
    lastAttempt.current = null;
    return () => {
      session.current++;
      requestInFlight.current = false;
    };
  }, [enabled]);

  const running = status?.run?.state === "running";

  const check = useCallback(async () => {
    if (!enabled || requestInFlight.current) return;
    const currentSession = session.current;
    requestInFlight.current = true;
    lastAttempt.current = Date.now();
    setChecking(true);
    try {
      const next = await selfUpdateApi.check();
      if (currentSession !== session.current) return;
      setStatus(next);
      setError(null);
    } catch (cause) {
      if (currentSession === session.current) setError((cause as Error).message);
    } finally {
      if (currentSession === session.current) {
        requestInFlight.current = false;
        setChecking(false);
      }
    }
  }, [enabled]);

  const apply = useCallback(async (tag?: string) => {
    if (!enabled || requestInFlight.current) return;
    const currentSession = session.current;
    requestInFlight.current = true;
    setApplying(true);
    try {
      const next = await selfUpdateApi.apply(tag);
      if (currentSession !== session.current) return;
      setStatus(next);
      setError(null);
    } catch (cause) {
      if (currentSession === session.current) setError((cause as Error).message);
    } finally {
      if (currentSession === session.current) {
        requestInFlight.current = false;
        setApplying(false);
      }
    }
  }, [enabled]);

  useEffect(() => {
    if (!enabled) return;
    let cancelled = false;
    requestInFlight.current = true;
    setLoading(true);
    void (async () => {
      let shouldCheck = false;
      try {
        const next = await selfUpdateApi.status();
        if (cancelled) return;
        setStatus(next);
        setError(null);
        shouldCheck = next.run?.state !== "running";
      } catch (cause) {
        if (!cancelled) setError((cause as Error).message);
      } finally {
        if (!cancelled) {
          requestInFlight.current = false;
          setLoading(false);
        }
      }
      if (!cancelled && shouldCheck) void check();
    })();
    return () => {
      cancelled = true;
    };
  }, [enabled, check]);

  // Discovery belongs to the workspace, so it continues outside Settings.
  // Hidden tabs wait until visible again; repeated focus events are throttled.
  useEffect(() => {
    if (!enabled || running) return;
    const checkIfDue = () => {
      if (document.visibilityState === "visible" &&
          selfUpdateService.checkDue(lastAttempt.current, Date.now())) {
        void check();
      }
    };
    const interval = window.setInterval(() => {
      if (document.visibilityState === "visible") void check();
    }, SELF_UPDATE_CHECK_INTERVAL_MS);
    document.addEventListener("visibilitychange", checkIfDue);
    window.addEventListener("online", checkIfDue);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", checkIfDue);
      window.removeEventListener("online", checkIfDue);
    };
  }, [enabled, running, check]);

  // While an update runs, keep polling. The updater restarts the backend, so
  // failed polls are expected mid-run — surface them as "restarting" instead
  // of an error and keep going until the new backend answers. Each answer also
  // asks which frontend it serves, so the page moves onto a new build as soon
  // as the restarted backend has one rather than at the next minute's check.
  useEffect(() => {
    if (!enabled || !running) return;
    const currentSession = session.current;
    const interval = window.setInterval(() => {
      if (requestInFlight.current) return;
      requestInFlight.current = true;
      void (async () => {
        try {
          const next = await selfUpdateApi.status();
          if (currentSession !== session.current) return;
          setStatus(next);
          setRestarting(false);
          void frontendBuildStore.getState().check();
        } catch {
          if (currentSession === session.current) setRestarting(true);
        } finally {
          if (currentSession === session.current) requestInFlight.current = false;
        }
      })();
    }, SELF_UPDATE_RUNNING_POLL_INTERVAL_MS);
    return () => window.clearInterval(interval);
  }, [enabled, running]);

  return { status, loading, checking, applying, restarting, error, check, apply };
}

export type SelfUpdateController = ReturnType<typeof useSelfUpdate>;
