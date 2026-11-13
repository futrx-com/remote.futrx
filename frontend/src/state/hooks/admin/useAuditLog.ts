import { useCallback, useEffect } from "preact/hooks";
import { useStore } from "zustand";
import type { StoreApi } from "zustand/vanilla";
import type { AuditEntry, AuditFilters, AuditLogState } from "../../../models/audit";
import type { AuditLogCommands, AuditLogStateActions } from "../../../port/audit";

export interface AuditLog {
  entries: AuditEntry[];
  filters: AuditFilters;
  loading: boolean;
  loadingMore: boolean;
  hasMore: boolean;
  error: string | null;
  exportUrl: string;
  setFilters: (filters: AuditFilters) => void;
  refresh: () => Promise<void>;
  loadMore: () => Promise<void>;
}

export function useAuditLog(
  enabled: boolean,
  dependencies: {
    store: StoreApi<AuditLogState & AuditLogStateActions>;
    commands: AuditLogCommands;
    exportUrl: (filters: AuditFilters) => string;
  },
): AuditLog {
  const { store, commands, exportUrl } = dependencies;
  const entries = useStore(store, (state) => state.entries);
  const filters = useStore(store, (state) => state.filters);
  const cursor = useStore(store, (state) => state.cursor);
  const loading = useStore(store, (state) => state.loading);
  const loadingMore = useStore(store, (state) => state.loadingMore);
  const error = useStore(store, (state) => state.error);
  const setFilters = useStore(store, (state) => state.setFilters);

  const refresh = useCallback(() => commands.refresh(filters), [commands, filters]);
  const loadMore = useCallback(
    () => commands.loadMore(filters, cursor, loadingMore),
    [commands, cursor, filters, loadingMore],
  );

  // Filter changes and enabling the view start a request from the same effect
  // as before; changing filters alone does not invalidate a pending response.
  useEffect(() => {
    if (enabled) void refresh();
  }, [enabled, refresh]);

  return {
    entries, filters, loading, loadingMore, error,
    hasMore: Boolean(cursor),
    exportUrl: exportUrl(filters),
    setFilters, refresh, loadMore,
  };
}
