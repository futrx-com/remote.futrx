import assert from "node:assert/strict";
import test from "node:test";
import { QuestionAnswerState } from "./questionAnswerState.ts";

test("question state preserves native IDs, legacy label fallback, and combined answers", () => {
  const questions = [
    { id: "pick", multiSelect: true, isOther: true, options: [{ id: "a", label: "Same" }, { id: "b", label: "Same" }] },
    { options: [{ label: "Legacy" }] },
  ];
  let state = new QuestionAnswerState();
  let rows = state.view(questions);
  assert.equal(state.complete(rows), false);
  state = state.select(rows[0], rows[0].options[0]);
  rows = state.view(questions);
  state = state.select(rows[0], rows[0].options[1]);
  state = state.enterText(rows[0], "  Custom  ");
  state = state.select(rows[1], rows[1].options[0]);
  rows = state.view(questions);
  assert.equal(state.complete(rows), true);
  assert.deepEqual(state.responses(rows), { pick: ["a", "b", "Custom"], 1: ["Legacy"] });
  state = state.select(rows[0], rows[0].options[0]);
  assert.deepEqual(state.responses(rows).pick, ["b", "Custom"]);
});

test("single choice and free text replace one another without mutating earlier state", () => {
  const questions = [{ id: "q", isOther: true, options: [{ id: "id", label: "Option" }] }];
  const empty = new QuestionAnswerState();
  const row = empty.view(questions)[0];
  const typed = empty.enterText(row, "  Other  ");
  const selected = typed.select(row, row.options[0]);
  assert.deepEqual(typed.responses([row]), { q: ["Other"] });
  assert.deepEqual(selected.responses([row]), { q: ["id"] });
  assert.equal(selected.view(questions)[0].other, "");
  const replaced = selected.enterText(row, "Replacement");
  assert.equal(replaced.view(questions)[0].options[0].selected, false);
  assert.deepEqual(replaced.responses([row]), { q: ["Replacement"] });
  assert.equal(replaced.enterText(row, "  ").complete([row]), false);
  assert.equal(empty.complete([]), false);
});

test("selection keeps the render snapshot semantics when events share a render", () => {
  const questions = [{ id: "q", multiSelect: true, options: [{ id: "a" }] }];
  const state = new QuestionAnswerState();
  const row = state.view(questions)[0];
  const twice = state.select(row, row.options[0]).select(row, row.options[0]);
  assert.deepEqual(twice.responses([row]), { q: ["a", "a"] });
});
