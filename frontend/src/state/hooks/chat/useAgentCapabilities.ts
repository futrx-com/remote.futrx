import { useStore } from "zustand";
import { useCallback, useEffect } from "preact/hooks";
import {
  agentCapabilityCatalogStore,
  selectAgentCapabilityCatalog,
} from "../../stores/agents/agentCapabilityCatalogStore";
import { useAuthContext } from "../../context/AuthContext";

export function useAgentCapabilities(projectId?: string) {
  const { auth } = useAuthContext();
  const userId = auth.email || auth.adminEmail || "anonymous";
  const scope = `${userId.trim().toLowerCase()}\0${projectId || "host"}`;
  const snapshot = useStore(
    agentCapabilityCatalogStore,
    selectAgentCapabilityCatalog(userId, projectId),
  );
  const observe = useStore(agentCapabilityCatalogStore, (state) => state.observe);
  const load = useStore(agentCapabilityCatalogStore, (state) => state.load);

  useEffect(() => {
    const unobserve = observe(userId, projectId);
    void load(userId, projectId).catch(() => undefined);
    return unobserve;
  }, [scope, userId, projectId, observe, load]);

  const refresh = useCallback(async () => {
    try {
      await load(userId, projectId, { force: true });
    } catch {
      // The store retains stale data and exposes the error in its snapshot.
    }
  }, [scope, userId, projectId, load]);

  return {
    catalog: snapshot.catalog,
    loading: snapshot.loading,
    refreshing: snapshot.refreshing,
    error: snapshot.error,
    refresh,
  };
}
