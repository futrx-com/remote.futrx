/** The browser subscription shape needed by account ownership policy. */
interface EndpointSubscription {
  endpoint: string;
}

/**
 * Keeps a browser endpoint only when the authenticated account owns it.
 * Verification failures deliberately fail closed to protect shared browsers.
 */
export async function reconcileSubscriptionOwnership<T extends EndpointSubscription>(
  subscription: T,
  ownsEndpoint: (endpoint: string) => Promise<boolean>,
  invalidateLocal: (subscription: T) => Promise<unknown>
): Promise<boolean> {
  try {
    if (await ownsEndpoint(subscription.endpoint)) return true;
  } catch {
    // The endpoint is untrusted until its account ownership is proven.
  }
  await invalidateLocal(subscription);
  return false;
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
