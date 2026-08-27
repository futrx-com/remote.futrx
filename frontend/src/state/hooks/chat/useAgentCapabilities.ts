import { useCallback, useEffect, useState } from "preact/hooks";
import {
  agentCapabilityCatalogStore,
  type AgentCapabilityCatalogSnapshot,
} from "../../agents/agentCapabilityCatalog";
import { useAuthContext } from "../../context/AuthContext";

export function useAgentCapabilities(projectId?: string) {
  const { auth } = useAuthContext();
  const userId = auth.email || auth.adminEmail || "anonymous";
  const scope = `${userId.trim().toLowerCase()}\0${projectId || "host"}`;
  const [state, setState] = useState<{
    scope: string;
    snapshot: AgentCapabilityCatalogSnapshot;
  }>(() => ({
    scope,
    snapshot: agentCapabilityCatalogStore.read(userId, projectId),
  }));
  const snapshot = state.scope === scope
    ? state.snapshot
    : agentCapabilityCatalogStore.read(userId, projectId);

  useEffect(() => {
    function sync() {
      setState({
        scope,
        snapshot: agentCapabilityCatalogStore.read(userId, projectId),
      });
    }
    const unsubscribe = agentCapabilityCatalogStore.subscribe(userId, projectId, sync);
    sync();
    void agentCapabilityCatalogStore.load(userId, projectId).catch(() => undefined);
    return unsubscribe;
  }, [scope, userId, projectId]);

  const refresh = useCallback(async () => {
    try {
      await agentCapabilityCatalogStore.load(userId, projectId, { force: true });
    } catch {
      // The store retains stale data and exposes the error in its snapshot.
    }
  }, [scope, userId, projectId]);

  return {
    catalog: snapshot.catalog,
    loading: snapshot.loading,
    refreshing: snapshot.refreshing,
    error: snapshot.error,
    refresh,
  };
}
