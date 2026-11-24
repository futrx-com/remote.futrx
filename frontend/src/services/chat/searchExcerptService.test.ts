import assert from "node:assert/strict";
import test from "node:test";
import { searchExcerptService } from "./searchExcerptService.ts";

const highlighted = (text: string, spans: { start: number; end: number }[]) =>
  spans.map((span) => text.slice(span.start, span.end));

test("an excerpt whose match is near the start is unchanged", () => {
  const out = searchExcerptService.focus("find the needle here", [{ start: 9, end: 15 }], 40);
  assert.equal(out.text, "find the needle here");
  assert.deepEqual(highlighted(out.text, out.spans), ["needle"]);
});

test("a late match moves to the front, starting at a word", () => {
  const text = "one two three four five six seven eight nine ten needle end";
  const start = text.indexOf("needle");
  const out = searchExcerptService.focus(text, [{ start, end: start + 6 }], 10);
  assert.equal(out.text, "… nine ten needle end");
  assert.deepEqual(highlighted(out.text, out.spans), ["needle"]);
});

test("every match keeps its place after trimming", () => {
  const text = "a".repeat(100) + " alpha beta alpha";
  const spans = [{ start: 101, end: 106 }, { start: 112, end: 117 }];
  const out = searchExcerptService.focus(text, spans, 5);
  assert.deepEqual(highlighted(out.text, out.spans), ["alpha", "alpha"]);
});

test("an excerpt without matches is unchanged", () => {
  assert.deepEqual(searchExcerptService.focus("plain", [], 5), { text: "plain", spans: [] });
});
