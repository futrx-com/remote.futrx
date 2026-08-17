import assert from "node:assert/strict";
import test from "node:test";
import type { AuditEntry, AuditFilters } from "../../models/audit.ts";
import { auditLogState } from "./auditLogState.ts";

const noFilters: AuditFilters = { actor: "", action: "", from: "", to: "" };

function entry(overrides: Partial<AuditEntry> = {}): AuditEntry {
  return {
    at: "2026-08-17T10:00:00Z",
    actor: { email: "admin@example.com" },
    action: "project.create",
    target: { type: "project", id: "p1" },
    ok: true,
    ...overrides,
  };
}

test("searchParams omits blank filters", () => {
  const params = auditLogState.searchParams(noFilters);
  assert.equal(params.toString(), "");
});

test("searchParams normalizes the actor and keeps the action prefix verbatim", () => {
  const params = auditLogState.searchParams({
    ...noFilters,
    actor: "  Admin@Example.com ",
    action: "project.secret.",
  });
  assert.equal(params.get("actor"), "admin@example.com");
  assert.equal(params.get("action"), "project.secret.");
});

test("searchParams converts local date bounds to absolute instants", () => {
  const params = auditLogState.searchParams({
    ...noFilters,
    from: "2026-08-01T00:00",
    to: "2026-08-31T23:59",
  });
  assert.equal(params.get("from"), new Date("2026-08-01T00:00").toISOString());
  assert.equal(params.get("to"), new Date("2026-08-31T23:59").toISOString());
});

test("searchParams ignores an unparseable date instead of failing the query", () => {
  const params = auditLogState.searchParams({ ...noFilters, from: "2026-08-" });
  assert.equal(params.has("from"), false);
});

test("searchParams carries paging options", () => {
  const params = auditLogState.searchParams(noFilters, { limit: 50, cursor: "2026-08:12" });
  assert.equal(params.get("limit"), "50");
  assert.equal(params.get("cursor"), "2026-08:12");
});

test("searchParams omits paging options that are absent or zero", () => {
  const params = auditLogState.searchParams(noFilters, { limit: 0, cursor: "" });
  assert.equal(params.has("limit"), false);
  assert.equal(params.has("cursor"), false);
});

test("appendPage adds an older page beneath the loaded entries", () => {
  const loaded = [entry({ at: "2026-08-17T12:00:00Z", action: "project.delete" })];
  const older = entry({ at: "2026-08-17T09:00:00Z" });

  const next = auditLogState.appendPage(loaded, { entries: [older] });

  assert.deepEqual(
    next.map((item) => item.action),
    ["project.delete", "project.create"],
  );
});

test("appendPage drops entries an overlapping page repeats", () => {
  const loaded = [entry()];
  const next = auditLogState.appendPage(loaded, { entries: [entry(), entry({ action: "auth.logout" })] });

  assert.deepEqual(
    next.map((item) => item.action),
    ["project.create", "auth.logout"],
  );
});

test("appendPage keeps the same array when a page adds nothing", () => {
  const loaded = [entry()];
  assert.equal(auditLogState.appendPage(loaded, { entries: [entry()] }), loaded);
  assert.equal(auditLogState.appendPage(loaded, { entries: [] }), loaded);
});

test("appendPage keeps two entries that share a timestamp but differ otherwise", () => {
  const loaded = [entry()];
  const sameInstant = entry({ action: "project.secret.read" });

  const next = auditLogState.appendPage(loaded, { entries: [sameInstant] });

  assert.equal(next.length, 2);
});

test("hasMore follows the cursor the server returned", () => {
  assert.equal(auditLogState.hasMore({ entries: [], nextCursor: "2026-08:4" }), true);
  assert.equal(auditLogState.hasMore({ entries: [entry()] }), false);
  assert.equal(auditLogState.hasMore(null), false);
});
