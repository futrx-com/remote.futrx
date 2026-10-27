import type { AuditEntry, AuditFilters, AuditPage } from "../../models/audit";

// The value shape a `datetime-local` input produces, with optional seconds.
const LOCAL_DATE_TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2})?$/;

// Query and paging rules for the admin audit log. The backend returns pages
// newest-first with an opaque cursor, so the only state worth pinning is how
// filters become query parameters and how a "load more" page is appended
// without duplicating or reordering what is already on screen.
class AuditLogState {
  // searchParams turns UI filters into the admin API's query string. Blank
  // fields are omitted so an empty filter means "no filter", and the local
  // `datetime-local` bounds are converted to absolute instants.
  searchParams(
    filters: AuditFilters,
    options: { limit?: number; cursor?: string } = {},
  ): URLSearchParams {
    const params = new URLSearchParams();
    const actor = filters.actor.trim().toLowerCase();
    const action = filters.action.trim();
    if (actor) params.set("actor", actor);
    if (action) params.set("action", action);
    const from = this.toInstant(filters.from);
    if (from) params.set("from", from);
    const to = this.toInstant(filters.to);
    if (to) params.set("to", to);
    if (options.limit) params.set("limit", String(options.limit));
    if (options.cursor) params.set("cursor", options.cursor);
    return params;
  }

  // toInstant converts a `datetime-local` value to an RFC3339 instant. Anything
  // that is not a complete local date and time means the bound is unset rather
  // than an error, because a half-typed date must not blank the table. The
  // shape is checked explicitly: Date parsing accepts partial strings, so
  // "2026-08-" would otherwise become a silent, wrong bound.
  toInstant(localDateTime: string): string {
    const trimmed = localDateTime.trim();
    if (!LOCAL_DATE_TIME.test(trimmed)) return "";
    const parsed = new Date(trimmed);
    if (Number.isNaN(parsed.getTime())) return "";
    return parsed.toISOString();
  }

  // appendPage adds an older page beneath what is already loaded. Entries the
  // list already holds are dropped: the log is append-only, so a repeated
  // entry means a page overlapped, never that something changed.
  appendPage(loaded: AuditEntry[], page: AuditPage): AuditEntry[] {
    const seen = new Set(loaded.map((entry) => this.identity(entry)));
    const added = page.entries.filter((entry) => !seen.has(this.identity(entry)));
    return added.length === 0 ? loaded : [...loaded, ...added];
  }

  // hasMore reports whether a "load more" button should be offered.
  hasMore(page: AuditPage | null): boolean {
    return Boolean(page?.nextCursor);
  }

  // identity is the de-duplication key: an append-only log can hold two
  // entries at the same instant, so the action and target take part too.
  identity(entry: AuditEntry): string {
    return [
      entry.at,
      entry.action,
      entry.actor.email ?? "",
      entry.target?.id ?? "",
      entry.ok ? "1" : "0",
    ].join("|");
  }
}

export const auditLogState = new AuditLogState();
