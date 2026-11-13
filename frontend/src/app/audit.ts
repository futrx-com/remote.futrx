import { auditApi } from "../api/auditApi";
import { AuditLogService } from "../services/admin/auditLogService";
import { createAuditLogStore } from "../state/stores/admin/auditLogStore";

/** Audit filters, results and in-flight request sequencing share one mount. */
export function createAuditLog() {
  const store = createAuditLogStore();
  return {
    store,
    commands: new AuditLogService(auditApi, store),
    exportUrl: auditApi.exportUrl,
  };
}
