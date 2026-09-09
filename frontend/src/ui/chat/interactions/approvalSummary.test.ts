import assert from "node:assert/strict";
import test from "node:test";
import { approvalSummaryFields } from "./approvalSummary.ts";

test("summarizes command, action, and reason in the requested order", () => {
  assert.deepEqual(
    approvalSummaryFields({
      reason: "The agent needs to update the repository.",
      action: "edit files",
      command: "git apply patch.diff",
    }),
    [
      { label: "Command", value: "git apply patch.diff" },
      { label: "Action", value: "edit files" },
      { label: "Reason", value: "The agent needs to update the repository." },
    ],
  );
});

test("uses supported fallback keys and ignores blank values", () => {
  assert.deepEqual(
    approvalSummaryFields({ cmd: "npm test", operation: "run tests", why: "  " }),
    [
      { label: "Command", value: "npm test" },
      { label: "Action", value: "run tests" },
    ],
  );
});

test("renders primitive and structured values without throwing", () => {
  assert.deepEqual(
    approvalSummaryFields({ command: ["git", "status"], action: true, reason: 0 }),
    [
      { label: "Command", value: '["git","status"]' },
      { label: "Action", value: "true" },
      { label: "Reason", value: "0" },
    ],
  );
});

test("returns no fields when the payload has no supported summary keys", () => {
  assert.deepEqual(approvalSummaryFields({ input: "raw" }), []);
});
