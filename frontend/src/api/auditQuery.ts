import type { AuditFilters } from "../models/audit.ts";

// The value shape a `datetime-local` input produces, with optional seconds.
const LOCAL_DATE_TIME = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2})?$/;

/** Serialization belongs to the server boundary, independently of UI state. */
class AuditQuery {
  // searchParams turns UI filters into the admin API's query string. Blank
  // fields are omitted so an empty filter means "no filter", and the local
  // `datetime-local` bounds are converted to absolute instants.
  searchParams(
    filters: AuditFilters,
    options: { limit?: number; cursor?: string } = {},
  ): URLSearchParams {
    const params = new URLSearchParams();
    if (filters.project?.trim()) params.set("project", filters.project.trim());
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
  private toInstant(localDateTime: string): string {
    const trimmed = localDateTime.trim();
    if (!LOCAL_DATE_TIME.test(trimmed)) return "";
    const parsed = new Date(trimmed);
    if (Number.isNaN(parsed.getTime())) return "";
    return parsed.toISOString();
  }

  exportParams(filters: AuditFilters): URLSearchParams {
    const params = new URLSearchParams();
    const from = this.toInstant(filters.from);
    if (from) params.set("from", from);
    const to = this.toInstant(filters.to);
    if (to) params.set("to", to);
    return params;
  }

}

export const auditQuery = new AuditQuery();
