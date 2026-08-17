import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { auditApi } from "../../../api/auditApi";
import { DEFAULT_AUDIT_LOG_LIMIT } from "../../../config/api";
import {
  EMPTY_AUDIT_FILTERS,
  type AuditEntry,
  type AuditFilters,
} from "../../../models/audit";
import { auditLogState } from "../../admin/auditLogState";

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

// useAuditLog owns one filtered view of the admin audit log. Filters are held
// here rather than applied client-side: the log can span months of monthly
// files, so narrowing happens on the server and paging resumes from the cursor
// it returns.
export function useAuditLog(enabled: boolean): AuditLog {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [filters, setFilters] = useState<AuditFilters>(EMPTY_AUDIT_FILTERS);
  const [cursor, setCursor] = useState<string>("");
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Every request gets a sequence number. A response is applied only if no
  // newer request started meanwhile, so quickly re-filtering cannot leave the
  // table showing a superseded page.
  const requestSeq = useRef(0);

  const refresh = useCallback(async () => {
    const seq = ++requestSeq.current;
    setLoading(true);
    setError(null);
    try {
      const page = await auditApi.list(filters, { limit: DEFAULT_AUDIT_LOG_LIMIT });
      if (seq !== requestSeq.current) return;
      setEntries(page.entries);
      setCursor(page.nextCursor ?? "");
    } catch (cause) {
      if (seq !== requestSeq.current) return;
      setError((cause as Error).message);
    } finally {
      if (seq === requestSeq.current) setLoading(false);
    }
  }, [filters]);

  const loadMore = useCallback(async () => {
    if (!cursor || loadingMore) return;
    const seq = ++requestSeq.current;
    setLoadingMore(true);
    setError(null);
    try {
      const page = await auditApi.list(filters, {
        limit: DEFAULT_AUDIT_LOG_LIMIT,
        cursor,
      });
      if (seq !== requestSeq.current) return;
      setEntries((current) => auditLogState.appendPage(current, page));
      setCursor(page.nextCursor ?? "");
    } catch (cause) {
      if (seq !== requestSeq.current) return;
      setError((cause as Error).message);
    } finally {
      if (seq === requestSeq.current) setLoadingMore(false);
    }
  }, [cursor, filters, loadingMore]);

  // Changing a filter restarts paging: a cursor is a position inside one
  // filtered range and means nothing in another.
  useEffect(() => {
    if (enabled) void refresh();
  }, [enabled, refresh]);

  return {
    entries,
    filters,
    loading,
    loadingMore,
    hasMore: Boolean(cursor),
    error,
    exportUrl: auditApi.exportUrl(filters),
    setFilters,
    refresh,
    loadMore,
  };
}
