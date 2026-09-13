/** The browser subscription shape needed by account ownership policy. */
export interface EndpointSubscription {
  endpoint: string;
}

/**
 * What the server said about one browser endpoint.
 *
 * `unverified` is deliberately distinct from `foreign`: "the server says this
 * belongs to somebody else" and "the server could not be reached" call for
 * opposite actions, and treating them alike is what silently unregistered
 * devices during every backend restart.
 */
export type SubscriptionOwnership = "owned" | "foreign" | "unverified";

/**
 * Keeps a browser endpoint unless the account that owns it is provably not the
 * signed-in one. An endpoint left behind by another account is invalidated
 * immediately; an unreachable server leaves the registration untouched, since
 * the service worker re-checks ownership before displaying any notification
 * and so nothing can leak while verification is pending.
 */
export async function reconcileSubscriptionOwnership<T extends EndpointSubscription>(
  subscription: T,
  ownsEndpoint: (endpoint: string) => Promise<boolean>,
  invalidateLocal: (subscription: T) => Promise<unknown>
): Promise<SubscriptionOwnership> {
  let owned: boolean;
  try {
    owned = await ownsEndpoint(subscription.endpoint);
  } catch {
    return "unverified";
  }
  if (owned) return "owned";

  await invalidateLocal(subscription);
  return "foreign";
}

/** Removes both halves of a subscription, always invalidating the browser. */
export async function revokeSubscriptionForLogout<T extends EndpointSubscription>(
  subscription: T,
  removeServer: (endpoint: string) => Promise<unknown>,
  invalidateLocal: (subscription: T) => Promise<unknown>
): Promise<void> {
  // Start both immediately: a slow server must not postpone invalidating the
  // browser endpoint that can display the previous account's notifications.
  const serverRemoval = removeServer(subscription.endpoint).then(
    () => ({ failed: false as const, error: undefined }),
    (error: unknown) => ({ failed: true as const, error })
  );
  await invalidateLocal(subscription);
  const result = await serverRemoval;
  if (result.failed) throw result.error;
}
