import assert from "node:assert/strict";
import test from "node:test";
import { stopRunningChat } from "./stopRunningChat.ts";

test("does nothing if the server already reports the run as stopped", async () => {
  let cancelled = false;
  await stopRunningChat({ isRunning: async () => false, sendCancel: () => { cancelled = true; return true; } });
  assert.equal(cancelled, false);
});

test("issues cancellation then polls until server releases its run lock", async () => {
  let checks = 0;
  const actions: string[] = [];
  await stopRunningChat({
    isRunning: async () => { checks += 1; actions.push("check"); return checks < 4; },
    sendCancel: () => { actions.push("cancel"); return true; },
    wait: async (ms) => { assert.equal(ms, 250); actions.push("wait"); },
  });
  assert.deepEqual(actions, ["check", "cancel", "check", "wait", "check", "wait", "check"]);
});

test("refuses to rewind when the stop command cannot be sent", async () => {
  await assert.rejects(stopRunningChat({ isRunning: async () => true, sendCancel: () => false }), /Unable to stop/);
});

test("times out when the run lock never clears", async () => {
  let time = 0;
  const waits: number[] = [];
  await assert.rejects(stopRunningChat({
    isRunning: async () => true,
    sendCancel: () => true,
    now: () => time,
    timeoutMs: 500,
    pollIntervalMs: 250,
    wait: async (ms) => { waits.push(ms); time += ms; },
  }), /still stopping/);
  assert.deepEqual(waits, [250, 250]);
});

test("propagates network errors without claiming the run stopped", async () => {
  await assert.rejects(stopRunningChat({
    isRunning: async () => { throw new Error("metadata unavailable"); },
    sendCancel: () => { throw new Error("should not send"); },
  }), /metadata unavailable/);
});
