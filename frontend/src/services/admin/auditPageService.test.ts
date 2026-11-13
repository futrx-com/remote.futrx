import assert from "node:assert/strict";
import test from "node:test";
import type { AuditEntry } from "../../models/audit.ts";
import { auditPageService } from "./auditPageService.ts";

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

test("appendPage adds an older page beneath the loaded entries", () => {
  const loaded = [entry({ at: "2026-08-17T12:00:00Z", action: "project.delete" })];
  const older = entry({ at: "2026-08-17T09:00:00Z" });

  const next = auditPageService.appendPage(loaded, { entries: [older] });

  assert.deepEqual(
    next.map((item) => item.action),
    ["project.delete", "project.create"],
  );
});

test("appendPage drops entries an overlapping page repeats", () => {
  const loaded = [entry()];
  const next = auditPageService.appendPage(loaded, { entries: [entry(), entry({ action: "auth.logout" })] });

  assert.deepEqual(
    next.map((item) => item.action),
    ["project.create", "auth.logout"],
  );
});

test("appendPage keeps the same array when a page adds nothing", () => {
  const loaded = [entry()];
  assert.equal(auditPageService.appendPage(loaded, { entries: [entry()] }), loaded);
  assert.equal(auditPageService.appendPage(loaded, { entries: [] }), loaded);
});

test("appendPage keeps two entries that share a timestamp but differ otherwise", () => {
  const loaded = [entry()];
  const sameInstant = entry({ action: "project.secret.read" });

  const next = auditPageService.appendPage(loaded, { entries: [sameInstant] });

  assert.equal(next.length, 2);
});
