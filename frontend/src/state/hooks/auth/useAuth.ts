import { useCallback, useEffect, useMemo, useState } from "preact/hooks";
import { fetchAuthSession } from "../../../api/authApi";
import { UNAUTHENTICATED_SESSION } from "../../../config/auth";
import type { AuthSession } from "../../../models/auth";

export interface AuthState extends AuthSession {
  loading: boolean;
  refresh: () => Promise<void>;
}

const initial: Omit<AuthState, "refresh"> = {
  ...UNAUTHENTICATED_SESSION,
  loading: true,
};

export function useAuth(): AuthState {
  ////////////////
  // Local State
  ////////////////
  const [state, setState] = useState<Omit<AuthState, "refresh">>(initial);

  ////////////////
  // Handlers
  ////////////////
  const refresh = useCallback(async () => {
    const session = await fetchAuthSession();
    setState({ ...session, loading: false });
  }, []);

  ////////////////
  // Effects
  ////////////////
  useEffect(() => { void refresh(); }, [refresh]);

  return useMemo(() => ({ ...state, refresh }), [state, refresh]);
}
