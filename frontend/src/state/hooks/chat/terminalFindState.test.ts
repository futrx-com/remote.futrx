import assert from "node:assert/strict";
import test from "node:test";
import { terminalFindState } from "./terminalFindState.ts";

test("an empty query is idle whatever the addon last reported", () => {
  assert.deepEqual(terminalFindState.status("", { index: 2, count: 5 }), { kind: "idle" });
});

test("a query with no results is empty", () => {
  assert.deepEqual(terminalFindState.status("npm", null), { kind: "empty" });
  assert.deepEqual(terminalFindState.status("npm", { index: -1, count: 0 }), { kind: "empty" });
});

test("the active match is reported one-based", () => {
  assert.deepEqual(terminalFindState.status("npm", { index: 0, count: 3 }), { kind: "matched", position: 1, total: 3 });
  assert.deepEqual(terminalFindState.status("npm", { index: 2, count: 3 }), { kind: "matched", position: 3, total: 3 });
});

test("past the highlight limit the position falls back to the first match", () => {
  assert.deepEqual(
    terminalFindState.status("a", { index: -1, count: 1500 }),
    { kind: "matched", position: 1, total: 1500 },
  );
});
