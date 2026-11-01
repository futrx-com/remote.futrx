import { requestJson } from "./apiRequest";
import { API_ROUTES } from "../config/routes";
import type { AuditFilters, AuditPage } from "../models/audit";
import { auditLogState } from "../state/admin/auditLogState";

export const auditApi = {
  list: (filters: AuditFilters, options: { limit?: number; cursor?: string } = {}) =>
    requestJson<AuditPage>(
      "GET",
      API_ROUTES.audit.collection(auditLogState.searchParams(filters, options).toString()),
    ),
  // The export is a streamed download, so it is a URL for an anchor rather
  // than a fetch: only the time range narrows it, and it always returns the
  // raw stored JSONL.
  exportUrl: (filters: AuditFilters) => {
    const params = new URLSearchParams();
    const from = auditLogState.toInstant(filters.from);
    if (from) params.set("from", from);
    const to = auditLogState.toInstant(filters.to);
    if (to) params.set("to", to);
    return API_ROUTES.audit.export(params.toString());
  },
};
