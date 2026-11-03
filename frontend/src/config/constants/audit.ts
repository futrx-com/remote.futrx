import type { AuditFilters } from "../../models/audit.ts";

export const EMPTY_AUDIT_FILTERS: AuditFilters = {
  actor: "",
  action: "",
  from: "",
  to: "",
};
