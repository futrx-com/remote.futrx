import assert from "node:assert/strict";
import test from "node:test";
import { SHELL_EXITED, afterClose, embedderOrigin, retryDelayMs } from "./reconnect.js";

test("a dropped connection is retried while the pane is visible", () => {
  for (const closeCode of [1000, 1001, 1006, 1011, 4001]) {
    assert.equal(afterClose({ closeCode, visible: true }), "retry", String(closeCode));
  }
});

test("a hidden pane waits instead of retrying", () => {
  assert.equal(afterClose({ closeCode: 1006, visible: false }), "wait");
});

test("an exited shell is never reconnected automatically", () => {
  assert.equal(afterClose({ closeCode: SHELL_EXITED, visible: true }), "ended");
  assert.equal(afterClose({ closeCode: SHELL_EXITED, visible: false }), "ended");
});

test("retries back off from one second to a ten second ceiling", () => {
  assert.deepEqual(
    [0, 1, 2, 3, 4, 5, 50].map(retryDelayMs),
    [1000, 2000, 4000, 8000, 10000, 10000, 10000],
  );
  assert.equal(retryDelayMs(-3), 1000);
});

test("status goes only to the Remote origin that owns the application host", () => {
  assert.equal(
    embedderOrigin({ protocol: "https:", host: "terminal--demo.remote.example.com" }),
    "https://remote.example.com",
  );
  assert.equal(
    embedderOrigin({ protocol: "http:", host: "terminal--demo.remote.test:8080" }),
    "http://remote.test:8080",
  );
  assert.equal(embedderOrigin({ protocol: "http:", host: "localhost:8843" }), null);
  assert.equal(embedderOrigin({ protocol: "https:", host: "plain.remote.example.com" }), null);
});
