import assert from "node:assert/strict";
import test from "node:test";
import { advanceStreamingTextState } from "./streamingTextState.ts";

test("keeps the first presentation mode while content begins and settles", () => {
  const started = advanceStreamingTextState(undefined, {
    text: "first", streaming: true, presentation: "blocks",
  });
  assert.equal(started.active, true);
  const continued = advanceStreamingTextState(started, {
    text: "first second", streaming: true, presentation: "tokens",
  });
  assert.equal(continued.presentation, "blocks");
  assert.equal(continued.active, true);
  assert.equal(advanceStreamingTextState(continued, { text: "first second", streaming: false }).active, false);
});

test("a new turn keeps the previous reply settled until its text changes", () => {
  const settled = advanceStreamingTextState(undefined, { text: "previous reply", streaming: false });
  const requested = advanceStreamingTextState(settled, { text: "previous reply", streaming: true });
  assert.equal(requested.active, false);
  assert.equal(advanceStreamingTextState(requested, { text: "previous reply", streaming: true }).active, false);
  assert.equal(advanceStreamingTextState(requested, { text: "new reply", streaming: true }).active, true);
});

test("hydrated text stays hydrated across later prop updates", () => {
  const started = advanceStreamingTextState(undefined, { text: "history", streaming: false, hydrated: true });
  const later = advanceStreamingTextState(started, { text: "history updated", streaming: true, presentation: "blocks" });
  assert.equal(later.hydrated, true);
  assert.equal(later.presentation, "tokens");
  assert.equal(later.text, "history");
});
