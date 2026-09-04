import assert from "node:assert/strict";
import test from "node:test";
import { diffService } from "./diffService.ts";

test("lines pairs a replacement and merges the runs around it", () => {
  assert.deepEqual(diffService.lines("a\nb\nc\n", "a\nx\nc\n"), [
    { value: "a\n" },
    { value: "b\n", removed: true },
    { value: "x\n", added: true },
    { value: "c\n" },
  ]);
  // Adjacent same-kind lines collapse into one part.
  assert.deepEqual(diffService.lines("a\nb\nc\nd\n", "a\nd\n"), [
    { value: "a\n" },
    { value: "b\nc\n", removed: true },
    { value: "d\n" },
  ]);
  assert.deepEqual(diffService.lines("keep\nkeep\n", "keep\nkeep\n"), [
    { value: "keep\nkeep\n" },
  ]);
});

test("lines treats an empty side as all-added or all-removed", () => {
  assert.deepEqual(diffService.lines("", "new\n"), [{ value: "new\n", added: true }]);
  assert.deepEqual(diffService.lines("gone\n", ""), [{ value: "gone\n", removed: true }]);
  assert.deepEqual(diffService.lines("", ""), []);
});
