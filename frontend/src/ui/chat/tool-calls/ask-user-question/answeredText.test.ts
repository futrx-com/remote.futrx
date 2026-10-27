import assert from "node:assert/strict";
import test from "node:test";
import { answeredText } from "./answeredText.ts";

test("parses each question with its answer", () => {
  assert.deepEqual(
    answeredText.parse("Q: Which database?\nA: Postgres\n\nQ: Add auth?\nA: Yes; with Google"),
    [
      { question: "Which database?", answer: "Postgres" },
      { question: "Add auth?", answer: "Yes; with Google" },
    ],
  );
});

test("keeps a question that spans lines together", () => {
  assert.deepEqual(answeredText.parse("Q: Pick one:\n- fast\n- cheap\nA: fast"), [
    { question: "Pick one:\n- fast\n- cheap", answer: "fast" },
  ]);
});

test("keeps an empty answer rather than dropping the question", () => {
  assert.deepEqual(answeredText.parse("Q: Anything else?\nA: "), [{ question: "Anything else?", answer: "" }]);
});

test("returns null for an older one-line preview or malformed text", () => {
  assert.equal(answeredText.parse("Database: Postgres · Auth: Yes"), null);
  assert.equal(answeredText.parse("Q: no answer line"), null);
  assert.equal(answeredText.parse(""), null);
});
