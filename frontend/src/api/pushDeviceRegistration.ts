import {
  reconcileSubscriptionOwnership,
  type EndpointSubscription,
} from "./pushSubscriptionOwnership.ts";

/** Where a device ended up after Remote tried to restore its registration. */
export type PushDeviceState =
  /** This device holds a subscription the signed-in account owns. */
  | "registered"
  /** A subscription is present but the server could not confirm it yet. */
  | "unverified"
  /** This device receives nothing, and nothing may be created without asking. */
  | "absent";

/** What this browser currently holds, and what it is allowed to hold. */
export interface PushDevice<T extends EndpointSubscription> {
  /** The subscription this browser currently holds, if any. */
  subscription: T | null;
  /** Whether this account already turned notifications on in this browser. */
  optedIn: boolean;
  /** True only when permission is already granted, so restoring never prompts. */
  permissionGranted: boolean;
}

/**
 * The only things the restore policy may do. Each one is a whole action rather
 * than a step of one, so the policy decides *whether* a device is registered
 * and never how a subscription reaches the server.
 */
export interface PushDevicePorts<T extends EndpointSubscription> {
  /** Whether the endpoint was signed with a key the server has since replaced. */
  isSignedWithRetiredKey: (subscription: T) => boolean;
  /** Server answer for one endpoint; rejects when the server cannot answer. */
  ownsEndpoint: (endpoint: string) => Promise<boolean>;
  /** Drops the browser's endpoint alone, leaving another account's record be. */
  invalidateLocally: (subscription: T) => Promise<unknown>;
  /** Removes this device's registration from the server and the browser. */
  discardRegistration: (subscription: T) => Promise<unknown>;
  /** Subscribes this browser and registers the result under the account. */
  createRegistration: () => Promise<unknown>;
}

/**
 * Restores what the user already agreed to, without ever asking again.
 *
 * A push subscription outlives neither a rotated VAPID key nor a push service
 * that retires an endpoint, and the server's record of it can disappear with a
 * restore of `DATA_DIR`. None of that is the user withdrawing consent, so when
 * this device is missing a usable subscription and permission is still granted,
 * a replacement is created and registered silently. A permission prompt only
 * ever comes from the user pressing "Turn on".
 */
export async function restoreDeviceRegistration<T extends EndpointSubscription>(
  device: PushDevice<T>,
  ports: PushDevicePorts<T>
): Promise<PushDeviceState> {
  const subscription = device.subscription;
  if (subscription) {
    if (ports.isSignedWithRetiredKey(subscription)) {
      // The push service can never deliver to it, so replace it rather than
      // leaving the device believing it is on.
      await ports.discardRegistration(subscription);
    } else {
      const ownership = await reconcileSubscriptionOwnership(
        subscription,
        ports.ownsEndpoint,
        ports.invalidateLocally
      );
      if (ownership === "owned") return "registered";
      // Keep an unconfirmed registration exactly as it is: the server being
      // unreachable — mid-update, or offline — is not the user turning
      // notifications off, and discarding it here would cost the subscription.
      if (ownership === "unverified") return "unverified";
      // "foreign": already invalidated locally. Fall through and mint one for
      // the account that is actually signed in.
    }
  }

  if (!device.optedIn || !device.permissionGranted) return "absent";

  await ports.createRegistration();
  return "registered";
}
