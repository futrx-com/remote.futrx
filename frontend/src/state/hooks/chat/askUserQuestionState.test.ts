import assert from "node:assert/strict";
import test from "node:test";
import { askUserQuestionState } from "./askUserQuestionState.ts";

test("Codex option questions preserve a selected option when notes are added", () => {
  const selected = askUserQuestionState.toggleOption(
    { selected: new Set(), freeformActive: false },
    1,
    false,
    true,
  );
  const withNotes = askUserQuestionState.activateFreeform(selected, false, true);

  assert.deepEqual([...withNotes.selected], [1]);
  assert.equal(withNotes.freeformActive, true);
});

test("legacy single-select Other remains mutually exclusive", () => {
  const selected = askUserQuestionState.toggleOption(
    { selected: new Set(), freeformActive: true },
    0,
    false,
    false,
  );
  assert.equal(selected.freeformActive, false);

  const other = askUserQuestionState.activateFreeform(selected, false, false);
  assert.deepEqual([...other.selected], []);
  assert.equal(other.freeformActive, true);
});

test("Codex single-select options can be deselected before a note-only answer", () => {
  const initial = { selected: new Set([0]), freeformActive: true };
  const deselected = askUserQuestionState.toggleOption(initial, 0, false, true);

  assert.deepEqual([...deselected.selected], []);
  assert.equal(deselected.freeformActive, true);
});
