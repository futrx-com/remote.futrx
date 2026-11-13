import { DEFAULT_AUDIT_LOG_LIMIT } from "../../config/api.ts";
import type { AuditFilters } from "../../models/audit.ts";
import type { AuditLogApi, AuditLogCommands, AuditLogStore } from "../../port/audit.ts";
import { auditPageService } from "./auditPageService.ts";

/** One mounted audit view; only the most recently started request may publish. */
export class AuditLogService implements AuditLogCommands {
  private readonly api: AuditLogApi;
  private readonly store: AuditLogStore;
  private requestSequence = 0;

  constructor(api: AuditLogApi, store: AuditLogStore) {
    this.api = api;
    this.store = store;
  }

  refresh = async (filters: AuditFilters): Promise<void> => {
    const sequence = ++this.requestSequence;
    this.store.getState().beginRefresh();
    try {
      const page = await this.api.list(filters, { limit: DEFAULT_AUDIT_LOG_LIMIT });
      if (sequence !== this.requestSequence) return;
      this.store.getState().receiveEntries(page.entries, page.nextCursor ?? "");
    } catch (cause) {
      if (sequence !== this.requestSequence) return;
      this.store.getState().failRequest((cause as Error).message);
    } finally {
      if (sequence === this.requestSequence) this.store.getState().finishRefresh();
    }
  };

  // These inputs describe the render that exposed the command. Reading current
  // loading state here would change repeated clicks before the next render.
  loadMore = async (filters: AuditFilters, cursor: string, loadingMore: boolean): Promise<void> => {
    if (!cursor || loadingMore) return;
    const sequence = ++this.requestSequence;
    this.store.getState().beginLoadMore();
    try {
      const page = await this.api.list(filters, { limit: DEFAULT_AUDIT_LOG_LIMIT, cursor });
      if (sequence !== this.requestSequence) return;
      const entries = auditPageService.appendPage(this.store.getState().entries, page);
      this.store.getState().receiveEntries(entries, page.nextCursor ?? "");
    } catch (cause) {
      if (sequence !== this.requestSequence) return;
      this.store.getState().failRequest((cause as Error).message);
    } finally {
      if (sequence === this.requestSequence) this.store.getState().finishLoadMore();
    }
  };
}
