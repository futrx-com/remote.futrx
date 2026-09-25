import assert from "node:assert/strict";
import test from "node:test";
import {
  PLAN_QUOTA_CLOCK_INTERVAL_MS,
  PLAN_QUOTA_POLL_INTERVAL_MS,
  PLAN_QUOTA_REQUEST_TIMEOUT_MS,
} from "../../../config/planQuota.ts";
import type { AccountQuota } from "../../../models/agentQuota.ts";
import { startPlanQuotaUpdates } from "./planQuotaUpdates.ts";

const NOW = 1_787_500_000_000;

test("refreshes serially, retains the last snapshot through failures, and accepts a real empty result", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "setInterval", "Date"], now: NOW });
  let calls = 0;
  let resolveRequest!: (value: AccountQuota[]) => void;
  let rejectRequest!: (reason: Error) => void;
  let settled = 0;
  const snapshots: AccountQuota[][] = [];
  const stop = startPlanQuotaUpdates({
    load: () => {
      calls++;
      return new Promise((resolve, reject) => {
        resolveRequest = resolve;
        rejectRequest = reject;
      });
    },
    onSnapshot: (snapshot) => snapshots.push(snapshot),
    onSettled: () => { settled++; },
    onClock: () => {},
  });
  t.after(stop);
  assert.equal(calls, 1);
  t.mock.timers.tick(1000);
  assert.equal(calls, 1);
  const reading: AccountQuota[] = [{ provider: "claude", accountId: "work" }];
  resolveRequest(reading);
  await Promise.resolve();
  assert.deepEqual(snapshots, [reading]);
  assert.equal(settled, 1);

  t.mock.timers.tick(PLAN_QUOTA_POLL_INTERVAL_MS - 1);
  assert.equal(calls, 1);
  t.mock.timers.tick(1);
  assert.equal(calls, 2);
  rejectRequest(new Error("temporarily unavailable"));
  await Promise.resolve();
  assert.deepEqual(snapshots, [reading]);
  assert.equal(settled, 2);

  t.mock.timers.tick(PLAN_QUOTA_POLL_INTERVAL_MS);
  assert.equal(calls, 3);
  resolveRequest([]);
  await Promise.resolve();
  assert.deepEqual(snapshots, [reading, []]);
});

test("clock ticks during a failed request and teardown aborts and suppresses late callbacks", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "setInterval", "Date"], now: NOW });
  let signal!: AbortSignal;
  let resolveRequest!: (value: AccountQuota[]) => void;
  let calls = 0;
  let settled = 0;
  const snapshots: AccountQuota[][] = [];
  const clocks: number[] = [];
  const stop = startPlanQuotaUpdates({
    load: (requestSignal) => {
      signal = requestSignal;
      calls++;
      return new Promise((resolve) => { resolveRequest = resolve; });
    },
    onSnapshot: (snapshot) => snapshots.push(snapshot),
    onSettled: () => { settled++; },
    onClock: (nowMs) => clocks.push(nowMs),
  });
  t.after(stop);

  t.mock.timers.tick(PLAN_QUOTA_CLOCK_INTERVAL_MS);
  assert.deepEqual(clocks, [NOW + PLAN_QUOTA_CLOCK_INTERVAL_MS]);
  assert.equal(calls, 1, "a non-cooperative request must not be overlapped");
  assert.equal(signal.aborted, true, "the request deadline must still abort fetch");
  stop();
  resolveRequest([{ provider: "claude", accountId: "work" }]);
  await Promise.resolve();
  t.mock.timers.tick(PLAN_QUOTA_POLL_INTERVAL_MS * 3);
  assert.equal(calls, 1);
  assert.equal(settled, 0);
  assert.deepEqual(snapshots, []);
  assert.equal(clocks.length, 1);
});

test("timed-out fetches settle loading and schedule a fresh request", async (t) => {
  t.mock.timers.enable({ apis: ["setTimeout", "setInterval", "Date"], now: NOW });
  let calls = 0;
  let settled = 0;
  const stop = startPlanQuotaUpdates({
    load: (signal) => {
      calls++;
      return new Promise((_resolve, reject) => {
        signal.addEventListener("abort", () => reject(signal.reason), { once: true });
      });
    },
    onSnapshot: () => assert.fail("a timed-out request must not update the snapshot"),
    onSettled: () => { settled++; },
    onClock: () => {},
  });
  t.after(stop);

  t.mock.timers.tick(PLAN_QUOTA_REQUEST_TIMEOUT_MS);
  await Promise.resolve();
  assert.equal(settled, 1);
  t.mock.timers.tick(PLAN_QUOTA_POLL_INTERVAL_MS);
  assert.equal(calls, 2);
  stop();
  await Promise.resolve();
  t.mock.timers.tick(PLAN_QUOTA_POLL_INTERVAL_MS * 3);
  assert.equal(calls, 2);
  assert.equal(settled, 1);
});
