import { isDefinitiveRejection } from "./apiError.ts";
import { pushApi } from "./pushApi";
import { pushServiceWorkerApi } from "./pushServiceWorkerApi";
import type { PushBlocker, PushSubscriptionPayload } from "../models/push";
import { webPushTransport } from "../transport/webPushTransport";
import {
  restoreDeviceRegistration,
  type PushDevicePorts,
  type PushDeviceState,
} from "./pushDeviceRegistration.ts";
import { revokeSubscriptionForLogout } from "./pushSubscriptionOwnership.ts";
import { pushDeviceOptIn } from "../shared/pushDeviceOptIn.ts";

class PushSubscriptionApi {
  /** Why this browser cannot subscribe, or null when it can. */
  blocker(serverEnabled: boolean): PushBlocker | null {
    if (!serverEnabled) return "server-disabled";
    if (!pushServiceWorkerApi.isSupported || !webPushTransport.isPushManagerSupported) {
      // Safari only exposes PushManager to installed web apps, so an iPhone in
      // the browser lands here and needs the home-screen hint.
      return webPushTransport.isIOS() && !webPushTransport.isStandalone()
        ? "install-required"
        : "unsupported";
    }
    if (!webPushTransport.isNotificationSupported) return "unsupported";
    if (webPushTransport.notificationPermission === "denied") return "denied";
    return null;
  }

  /**
   * Brings this device back to whatever the account already asked for, and
   * runs on every authenticated app boot — not only when Settings happens to
   * open. A subscription the browser or a deploy invalidated is replaced
   * silently; one belonging to another account is dropped; one the server
   * cannot confirm right now is left alone.
   */
  async ensureRegistered(account: string, publicKey?: string): Promise<PushDeviceState> {
    if (!account || !this.#browserCanSubscribe()) return "absent";
    const registration = await this.#registration();
    if (!registration) return "absent";

    const existing = await webPushTransport.currentSubscription(registration);
    const optedIn = await pushDeviceOptIn.has(account);
    // A device with nothing registered that was never asked to receive
    // anything needs no server round trip at all.
    if (!existing && !optedIn) return "absent";

    const serverKey = publicKey ?? (await this.#fetchServerKey());
    if (!serverKey) return "absent";

    try {
      return await restoreDeviceRegistration(
        {
          subscription: existing,
          optedIn,
          // Restoring must never surface a prompt: only "Turn on" may do that.
          permissionGranted: webPushTransport.notificationPermission === "granted",
        },
        this.#restorePorts(registration, serverKey)
      );
    } catch (cause) {
      // The server refused the re-created device outright — the account is at
      // its device cap, or the browser produced keys the server rejects.
      // Retrying on every boot would unsubscribe and resubscribe forever, and
      // on Safari each unsubscribe can cost the notification permission
      // itself, so stop asking. Pressing "Turn on" still surfaces this same
      // error to the user.
      if (isDefinitiveRejection(cause)) await pushDeviceOptIn.forget(account);
      // A failed restore must not otherwise read as "the user turned this
      // off". Report what the device still holds and try again on the next
      // boot or focus.
      const remaining = await webPushTransport
        .currentSubscription(registration)
        .catch(() => null);
      return remaining ? "unverified" : "absent";
    }
  }

  /**
   * Asks for permission, subscribes this device, and registers it with the
   * server. Must be called from a user gesture — iOS rejects it otherwise.
   */
  async enable(account: string, publicKey: string): Promise<void> {
    // Ask first, before any await. Safari ties requestPermission to the user
    // activation that triggered it, and awaiting anything first spends it.
    const permission = await webPushTransport.requestPermission();
    if (permission !== "granted") {
      throw new Error(
        permission === "denied"
          ? "Notifications are blocked for this site. Allow them in your browser settings, then try again."
          : "Notification permission was dismissed."
      );
    }

    const registration = await pushServiceWorkerApi.register();
    if (!registration) throw new Error("This browser cannot register a service worker.");
    // Registration only queues the install; PushManager needs an active worker.
    await pushServiceWorkerApi.ready();

    const existing = await webPushTransport.currentSubscription(registration);
    // A stored subscription signed with a previous server key can never be
    // delivered to, so replace it rather than reporting success.
    if (existing && webPushTransport.isSignedWithRetiredKey(existing, publicKey)) {
      await webPushTransport.unsubscribe(existing);
    }

    const subscription =
      (await webPushTransport.currentSubscription(registration)) ??
      (await webPushTransport.subscribe(registration, publicKey));

    await pushApi.subscribe(this.#payload(subscription));
    // Remembered last: only a registration that reached the server should be
    // restored without asking.
    await pushDeviceOptIn.remember(account);
  }

  /** Removes this device, both locally and on the server. */
  async disable(account: string): Promise<void> {
    // Forget the opt-in first, so a restore racing this cannot resurrect the
    // subscription the user just asked to be rid of.
    await pushDeviceOptIn.forget(account);
    const subscription = await this.#currentSubscription();
    if (!subscription) return;
    // Tell the server first: if unsubscribing locally succeeded but the server
    // kept the endpoint, it would keep pushing to a dead registration.
    await pushApi.unsubscribe(subscription.endpoint);
    await webPushTransport.unsubscribe(subscription);
  }

  /** Revokes this browser on both sides before its session cookie is cleared. */
  async prepareForLogout(account: string): Promise<void> {
    await pushDeviceOptIn.forget(account);
    const subscription = await this.#currentSubscription();
    if (!subscription) return;

    await revokeSubscriptionForLogout(
      subscription,
      (endpoint) => pushApi.unsubscribe(endpoint),
      (candidate) => webPushTransport.unsubscribe(candidate)
    );
  }

  /** What restoring one device is allowed to do to this browser and account. */
  #restorePorts(
    registration: ServiceWorkerRegistration,
    serverKey: string
  ): PushDevicePorts<PushSubscription> {
    return {
      isSignedWithRetiredKey: (subscription) =>
        webPushTransport.isSignedWithRetiredKey(subscription, serverKey),
      ownsEndpoint: async (endpoint) => (await pushApi.subscriptionStatus(endpoint)).owned,
      invalidateLocally: (subscription) => webPushTransport.unsubscribe(subscription),
      discardRegistration: async (subscription) => {
        // The server first: an endpoint it kept would keep being pushed to.
        await pushApi.unsubscribe(subscription.endpoint).catch(() => undefined);
        await webPushTransport.unsubscribe(subscription);
      },
      createRegistration: async () => {
        const created = await webPushTransport.subscribe(registration, serverKey);
        await pushApi.subscribe(this.#payload(created));
      },
    };
  }

  /** Whether this browser exposes the APIs a subscription needs at all. */
  #browserCanSubscribe(): boolean {
    return (
      pushServiceWorkerApi.isSupported &&
      webPushTransport.isPushManagerSupported &&
      webPushTransport.isNotificationSupported
    );
  }

  async #fetchServerKey(): Promise<string> {
    try {
      const config = await pushApi.config();
      return config.enabled ? config.publicKey : "";
    } catch {
      return "";
    }
  }

  #payload(subscription: PushSubscription): PushSubscriptionPayload {
    // PushManager-created subscriptions contain the endpoint and both keys.
    // Keep the browser's serialized object intact at the server boundary.
    return subscription.toJSON() as PushSubscriptionPayload;
  }

  /**
   * The worker backing this origin, registering it when the browser has none.
   * A dropped registration takes the push subscription with it, so re-creating
   * it is the first half of restoring a device.
   */
  async #registration(): Promise<ServiceWorkerRegistration | null> {
    if (!pushServiceWorkerApi.isSupported) return null;
    const existing = await pushServiceWorkerApi.currentRegistration();
    if (existing) return existing;
    const registered = await pushServiceWorkerApi.register();
    if (!registered) return null;
    // PushManager needs an active worker, not merely a queued install.
    await pushServiceWorkerApi.ready();
    return registered;
  }

  async #currentSubscription(): Promise<PushSubscription | null> {
    const registration = await pushServiceWorkerApi.currentRegistration();
    if (!registration) return null;
    return webPushTransport.currentSubscription(registration);
  }
}

export const pushSubscriptionApi = new PushSubscriptionApi();
