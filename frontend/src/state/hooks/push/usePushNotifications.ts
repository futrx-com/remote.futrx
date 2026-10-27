import { useCallback, useEffect, useState } from "preact/hooks";
import { pushApi } from "../../../api/pushApi";
import { pushSubscriptionApi } from "../../../api/pushSubscriptionApi";
import type { PushDeviceState } from "../../../api/pushDeviceRegistration.ts";
import type { PushBlocker, PushStatus } from "../../../models/push";

export interface PushNotifications {
  status: PushStatus;
  blocker: PushBlocker | null;
  busy: boolean;
  testing: boolean;
  error: string | null;
  notice: string | null;
  enable: () => Promise<void>;
  disable: () => Promise<void>;
  sendTest: () => Promise<void>;
}

export function usePushNotifications(active: boolean, account: string): PushNotifications {
  const [status, setStatus] = useState<PushStatus>("loading");
  const [blocker, setBlocker] = useState<PushBlocker | null>(null);
  const [publicKey, setPublicKey] = useState("");
  const [busy, setBusy] = useState(false);
  const [testing, setTesting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const config = await pushApi.config();
      setPublicKey(config.publicKey);

      const reason = pushSubscriptionApi.blocker(config.enabled);
      setBlocker(reason);
      if (reason) {
        setStatus("blocked");
        return;
      }
      // The server's record and this device's registration can disagree — a
      // second device, or a subscription the browser dropped — so trust the
      // browser for what *this* device receives. A registration this account
      // asked for is restored here rather than reported as off.
      setStatus(statusOf(await pushSubscriptionApi.ensureRegistered(account, config.publicKey)));
    } catch (cause) {
      setStatus("blocked");
      setError(messageOf(cause));
    }
  }, [account]);

  useEffect(() => {
    if (!active) return;
    void refresh();
  }, [active, refresh]);

  const enable = useCallback(async () => {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await pushSubscriptionApi.enable(account, publicKey);
      setStatus("on");
      setNotice("This device will now receive notifications.");
    } catch (cause) {
      setError(messageOf(cause));
      // A refused prompt turns into a blocker, not a retryable error.
      await refresh();
    } finally {
      setBusy(false);
    }
  }, [account, publicKey, refresh]);

  const disable = useCallback(async () => {
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await pushSubscriptionApi.disable(account);
      setStatus("off");
    } catch (cause) {
      setError(messageOf(cause));
    } finally {
      setBusy(false);
    }
  }, [account]);

  const sendTest = useCallback(async () => {
    setTesting(true);
    setError(null);
    setNotice(null);
    try {
      await pushApi.test();
      setNotice("Test sent. It should arrive in a few seconds.");
    } catch (cause) {
      setError(messageOf(cause));
    } finally {
      setTesting(false);
    }
  }, []);

  return { status, blocker, busy, testing, error, notice, enable, disable, sendTest };
}

/**
 * A device the server could not confirm still holds its subscription, so it
 * reads as on: reporting off would tell the user to press a button that asks
 * for a permission they already granted.
 */
function statusOf(device: PushDeviceState): PushStatus {
  return device === "absent" ? "off" : "on";
}

function messageOf(cause: unknown): string {
  return cause instanceof Error ? cause.message : String(cause);
}
