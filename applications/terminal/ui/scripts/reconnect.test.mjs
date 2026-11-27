import assert from "node:assert/strict";
import test from "node:test";
import { afterFailure, retryDelayMs } from "./reconnect.js";

test("a lost connection is retried while the pane is visible", () => {
  assert.equal(afterFailure({ visible: true }), "retry");
});

test("a hidden pane waits instead of retrying", () => {
  assert.equal(afterFailure({ visible: false }), "wait");
});

test("an exited shell is never reconnected automatically", () => {
  assert.equal(afterFailure({ ended: true, visible: true }), "ended");
  assert.equal(afterFailure({ ended: true, visible: false }), "ended");
});

test("retries back off from one second to a ten second ceiling", () => {
  assert.deepEqual(
    [0, 1, 2, 3, 4, 5, 50].map(retryDelayMs),
    [1000, 2000, 4000, 8000, 10000, 10000, 10000],
  );
  assert.equal(retryDelayMs(-3), 1000);
});
