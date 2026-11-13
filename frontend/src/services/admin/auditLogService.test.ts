import assert from "node:assert/strict";
import test from "node:test";
import { DEFAULT_AUDIT_LOG_LIMIT } from "../../config/api.ts";
import type { AuditEntry, AuditFilters, AuditPage } from "../../models/audit.ts";
import { createAuditLogStore } from "../../state/stores/admin/auditLogStore.ts";
import { AuditLogService } from "./auditLogService.ts";

const filters: AuditFilters = { actor: "", action: "", from: "", to: "" };

function entry(action: string): AuditEntry {
  return { at: "2026-08-17T10:00:00Z", actor: {}, action, ok: true };
}

function setup() {
  const calls: {
    filters: AuditFilters;
    options: { limit: number; cursor?: string };
    resolve: (page: AuditPage) => void;
    reject: (cause: Error) => void;
  }[] = [];
  const store = createAuditLogStore();
  const service = new AuditLogService({
    list: (filters, options) => new Promise<AuditPage>((resolve, reject) => {
      calls.push({ filters, options, resolve, reject });
    }),
  }, store);
  return { calls, store, service };
}

test("latest refresh wins and an older rejection cannot overwrite its result", async () => {
  const { calls, store, service } = setup();
  const older = service.refresh(filters);
  const nextFilters = { ...filters, actor: "admin@example.com" };
  const latest = service.refresh(nextFilters);
  assert.equal(store.getState().loading, true);
  assert.equal(calls[0].filters, filters);
  assert.equal(calls[1].filters, nextFilters);
  assert.deepEqual(calls[1].options, { limit: DEFAULT_AUDIT_LOG_LIMIT });
  const entries = [entry("latest")];
  calls[1].resolve({ entries, nextCursor: "2026-08:2" });
  await latest;
  calls[0].reject(new Error("stale failure"));
  await older;
  assert.equal(store.getState().entries, entries);
  assert.equal(store.getState().cursor, "2026-08:2");
  assert.equal(store.getState().error, null);
  assert.equal(store.getState().loading, false);
});

test("repeated load-more clicks use captured render inputs and append only the latest page", async () => {
  const { calls, store, service } = setup();
  const loaded = entry("loaded");
  const olderEntry = entry("older");
  store.getState().receiveEntries([loaded], "2026-08:2");
  const first = service.loadMore(filters, "2026-08:2", false);
  const second = service.loadMore(filters, "2026-08:2", false);
  assert.equal(store.getState().loadingMore, true);
  assert.equal(calls.length, 2);
  assert.deepEqual(calls[1].options, { limit: DEFAULT_AUDIT_LOG_LIMIT, cursor: "2026-08:2" });
  // Guarded calls neither start a request nor invalidate the pending page.
  await service.loadMore(filters, "", false);
  await service.loadMore(filters, "2026-08:2", true);
  assert.equal(calls.length, 2);
  calls[1].resolve({ entries: [loaded, olderEntry] });
  await second;
  calls[0].resolve({ entries: [entry("stale")], nextCursor: "stale-cursor" });
  await first;
  assert.deepEqual(store.getState().entries, [loaded, olderEntry]);
  assert.equal(store.getState().cursor, "");
  assert.equal(store.getState().loadingMore, false);
});

test("superseded request kinds retain their existing loading flag", async () => {
  for (const firstKind of ["refresh", "loadMore"] as const) {
    const { calls, store, service } = setup();
    const startRefresh = () => service.refresh(filters);
    const startMore = () => service.loadMore(filters, "2026-08:2", false);
    const first = firstKind === "refresh" ? startRefresh() : startMore();
    const latest = firstKind === "refresh" ? startMore() : startRefresh();
    calls[1].resolve({ entries: [entry("latest")] });
    await latest;
    calls[0].resolve({ entries: [entry("stale")] });
    await first;
    assert.equal(store.getState().loading, firstKind === "refresh");
    assert.equal(store.getState().loadingMore, firstKind === "loadMore");
    assert.equal(store.getState().entries[0].action, "latest");
  }
});

test("editing filters alone preserves the pending request until the view starts another", async () => {
  const { calls, store, service } = setup();
  const pending = service.refresh(filters);
  const nextFilters = { ...filters, action: "project." };
  store.getState().setFilters(nextFilters);
  assert.equal(calls.length, 1);
  calls[0].resolve({ entries: [entry("previous-filter-result")] });
  await pending;
  assert.equal(store.getState().filters, nextFilters);
  assert.equal(store.getState().entries[0].action, "previous-filter-result");
});

test("current failures resolve with the error while retaining entries and cursor", async () => {
  for (const kind of ["refresh", "loadMore"] as const) {
    const { calls, store, service } = setup();
    const entries = [entry("loaded")];
    store.getState().receiveEntries(entries, "2026-08:2");
    store.getState().failRequest("previous error");
    const pending = kind === "refresh"
      ? service.refresh(filters)
      : service.loadMore(filters, "2026-08:2", false);
    assert.equal(store.getState().error, null);
    calls[0].reject(new Error("request failed"));
    await pending;
    assert.equal(store.getState().error, "request failed");
    assert.equal(store.getState().entries, entries);
    assert.equal(store.getState().cursor, "2026-08:2");
    assert.equal(store.getState().loading, false);
    assert.equal(store.getState().loadingMore, false);
  }
});

test("separate mounted audit views keep their state and request sequence independent", async () => {
  const first = setup();
  const second = setup();
  const pending = first.service.refresh(filters);
  assert.equal(second.store.getState().loading, false);
  assert.deepEqual(second.store.getState().entries, []);
  first.calls[0].resolve({ entries: [entry("first-view")] });
  await pending;
  assert.deepEqual(second.store.getState().entries, []);
  assert.equal(second.calls.length, 0);
});
