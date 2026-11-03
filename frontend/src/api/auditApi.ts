import { requestJson } from "./apiRequest";
import { API_ROUTES } from "../config/routes";
import type { AuditFilters, AuditPage } from "../models/audit";
import { auditQuery } from "./auditQuery";

export const auditApi = {
  list: (filters: AuditFilters, options: { limit?: number; cursor?: string } = {}) =>
    requestJson<AuditPage>(
      "GET",
      API_ROUTES.audit.collection(auditQuery.searchParams(filters, options).toString()),
    ),
  // The export is a streamed download, so it is a URL for an anchor rather
  // than a fetch: only the time range narrows it, and it always returns the
  // audit entries as JSONL.
  exportUrl: (filters: AuditFilters) => {
    const params = auditQuery.exportParams(filters);
    return API_ROUTES.audit.export(params.toString());
  },
};
