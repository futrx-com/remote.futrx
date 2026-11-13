import type { AuditEntry, AuditFilters, AuditPage } from "../models/audit.ts";

export interface AuditLogApi {
  list(filters: AuditFilters, options: { limit: number; cursor?: string }): Promise<AuditPage>;
}

export interface AuditLogStateActions {
  setFilters(filters: AuditFilters): void;
  beginRefresh(): void;
  beginLoadMore(): void;
  receiveEntries(entries: AuditEntry[], cursor: string): void;
  failRequest(message: string): void;
  finishRefresh(): void;
  finishLoadMore(): void;
}

export interface AuditLogStore {
  getState(): AuditLogStateActions & { entries: AuditEntry[] };
}

export interface AuditLogCommands {
  refresh(filters: AuditFilters): Promise<void>;
  loadMore(filters: AuditFilters, cursor: string, loadingMore: boolean): Promise<void>;
}
