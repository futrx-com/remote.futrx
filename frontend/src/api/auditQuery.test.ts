import assert from "node:assert/strict";
import test from "node:test";
import type { AuditFilters } from "../models/audit.ts";
import { auditQuery } from "./auditQuery.ts";

const noFilters: AuditFilters = { actor: "", action: "", from: "", to: "" };

test("searchParams omits blank filters", () => {
  const params = auditQuery.searchParams(noFilters);
  assert.equal(params.toString(), "");
});

test("searchParams normalizes the actor and keeps the action prefix verbatim", () => {
  const params = auditQuery.searchParams({
    ...noFilters,
    actor: "  Admin@Example.com ",
    action: "project.secret.",
  });
  assert.equal(params.get("actor"), "admin@example.com");
  assert.equal(params.get("action"), "project.secret.");
});

test("searchParams converts local date bounds to absolute instants", () => {
  const params = auditQuery.searchParams({
    ...noFilters,
    from: "2026-08-01T00:00",
    to: "2026-08-31T23:59",
  });
  assert.equal(params.get("from"), new Date("2026-08-01T00:00").toISOString());
  assert.equal(params.get("to"), new Date("2026-08-31T23:59").toISOString());
});

test("searchParams ignores an unparseable date instead of failing the query", () => {
  const params = auditQuery.searchParams({ ...noFilters, from: "2026-08-" });
  assert.equal(params.has("from"), false);
});

test("searchParams carries paging options", () => {
  const params = auditQuery.searchParams(noFilters, { limit: 50, cursor: "2026-08:12" });
  assert.equal(params.get("limit"), "50");
  assert.equal(params.get("cursor"), "2026-08:12");
});

test("searchParams omits paging options that are absent or zero", () => {
  const params = auditQuery.searchParams(noFilters, { limit: 0, cursor: "" });
  assert.equal(params.has("limit"), false);
  assert.equal(params.has("cursor"), false);
});
