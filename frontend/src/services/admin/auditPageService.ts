import type { AuditEntry, AuditPage } from "../../models/audit.ts";

class AuditPageService {
  // appendPage adds an older page beneath what is already loaded. Entries the
  // list already holds are dropped: the log is append-only, so a repeated
  // entry means a page overlapped, never that something changed.
  appendPage(loaded: AuditEntry[], page: AuditPage): AuditEntry[] {
    const seen = new Set(loaded.map((entry) => this.identity(entry)));
    const added = page.entries.filter((entry) => !seen.has(this.identity(entry)));
    return added.length === 0 ? loaded : [...loaded, ...added];
  }

  // identity is the de-duplication key: an append-only log can hold two
  // entries at the same instant, so the action and target take part too.
  private identity(entry: AuditEntry): string {
    return [
      entry.at,
      entry.action,
      entry.actor.email ?? "",
      entry.target?.id ?? "",
      entry.ok ? "1" : "0",
    ].join("|");
  }
}

export const auditPageService = new AuditPageService();
