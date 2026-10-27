import assert from "node:assert/strict";
import test from "node:test";
import { SELF_UPDATE_CHECK_INTERVAL_MS } from "../../config/server.ts";
import type { SelfUpdateStatus } from "../../models/selfUpdate";
import { selfUpdateService } from "./selfUpdateService.ts";

const available: SelfUpdateStatus = {
  currentVersion: "0.25.0",
  lastCheck: { checkedAt: 1, latestTag: "0.25.1", updateAvailable: true },
};

test("only announces a confirmed newer release", () => {
  assert.equal(selfUpdateService.availableTag(available), "0.25.1");
  assert.equal(selfUpdateService.availableTag(null), null);
  assert.equal(selfUpdateService.availableTag({ currentVersion: "0.25.0" }), null);
  for (const lastCheck of [
    { checkedAt: 1, updateAvailable: false, latestTag: "0.25.0" },
    { checkedAt: 1, updateAvailable: true },
    { checkedAt: 1, updateAvailable: true, latestTag: "" },
    { ...available.lastCheck!, error: "origin unavailable" },
  ]) {
    assert.equal(selfUpdateService.availableTag({ ...available, lastCheck }), null);
  }
});

test("hides the notice during an update and permits a failed update to be retried", () => {
  const run = { state: "running" as const, target: "0.25.1", startedAt: 1 };
  assert.equal(selfUpdateService.availableTag({ ...available, run }), null);
  assert.equal(selfUpdateService.availableTag({ ...available, run: { ...run, state: "failed" } }), "0.25.1");
  assert.equal(selfUpdateService.availableTag({
    currentVersion: "0.25.1",
    lastCheck: { checkedAt: 2, latestTag: "0.25.1", updateAvailable: false },
    run: { ...run, state: "succeeded" },
  }), null);
});

test("returning to a tab checks only when the previous attempt is at least an hour old", () => {
  const now = 2 * SELF_UPDATE_CHECK_INTERVAL_MS;
  assert.equal(selfUpdateService.checkDue(null, now), true);
  assert.equal(selfUpdateService.checkDue(now, now), false);
  assert.equal(selfUpdateService.checkDue(now - SELF_UPDATE_CHECK_INTERVAL_MS + 1, now), false);
  assert.equal(selfUpdateService.checkDue(now - SELF_UPDATE_CHECK_INTERVAL_MS, now), true);
});
