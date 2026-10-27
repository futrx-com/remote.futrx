import { useCallback } from "preact/hooks";

import { pushSubscriptionApi } from "../../../api/pushSubscriptionApi";

/** Revokes this browser's push endpoint before ending the current session. */
export function useAccountSignOut(account: string): () => void {
  return useCallback(() => {
    void pushSubscriptionApi
      .prepareForLogout(account)
      .catch(() => {})
      .then(() => window.location.assign("/auth/logout"));
  }, [account]);
}
