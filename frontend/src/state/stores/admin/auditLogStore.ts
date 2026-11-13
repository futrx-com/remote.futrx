import { createStore } from "zustand/vanilla";
import { EMPTY_AUDIT_FILTERS } from "../../../config/constants/audit.ts";
import type { AuditLogState } from "../../../models/audit.ts";
import type { AuditLogStateActions } from "../../../port/audit.ts";

export function createAuditLogStore() {
  return createStore<AuditLogState & AuditLogStateActions>()((set) => ({
    entries: [],
    filters: EMPTY_AUDIT_FILTERS,
    cursor: "",
    loading: false,
    loadingMore: false,
    error: null,
    setFilters: (filters) => set({ filters }),
    beginRefresh: () => set({ loading: true, error: null }),
    beginLoadMore: () => set({ loadingMore: true, error: null }),
    receiveEntries: (entries, cursor) => set({ entries, cursor }),
    failRequest: (error) => set({ error }),
    finishRefresh: () => set({ loading: false }),
    finishLoadMore: () => set({ loadingMore: false }),
  }));
}
