export interface AuditActor {
  email?: string;
  sub?: string;
  isAdmin?: boolean;
}

export interface AuditTarget {
  type?: string;
  id?: string;
  name?: string;
}

export interface AuditEntry {
  // RFC3339 timestamp, as written to the JSONL file.
  at: string;
  actor: AuditActor;
  action: string;
  target?: AuditTarget;
  ip?: string;
  userAgent?: string;
  meta?: Record<string, unknown>;
  ok: boolean;
  error?: string;
}

export interface AuditPage {
  entries: AuditEntry[];
  // Absent once the filtered range is exhausted.
  nextCursor?: string;
}

// AuditFilters mirrors the query the admin API accepts. `from` and `to` are
// `datetime-local` values (local wall clock, no zone) as typed in the UI.
export interface AuditFilters {
  actor: string;
  action: string;
  from: string;
  to: string;
}

export const EMPTY_AUDIT_FILTERS: AuditFilters = {
  actor: "",
  action: "",
  from: "",
  to: "",
};
