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

test("prefers Codex command actions over a shell wrapper", () => {
  assert.deepEqual(
    approvalSummaryFields({
      command: "/bin/bash -lc 'ls -la /workspace'",
      commandActions: [
        {
          type: "listFiles",
          command: "ls -la /workspace",
          path: "workspace",
        },
      ],
      reason: "Inspect the workspace contents.",
    }),
    [
      { label: "Command", value: "ls -la /workspace" },
      { label: "Action", value: "List files in workspace" },
      { label: "Reason", value: "Inspect the workspace contents." },
    ],
  );
});

test("summarizes every parsed command action", () => {
  assert.deepEqual(
    approvalSummaryFields({
      command: "/bin/bash -lc 'cat README.md | rg install'",
      commandActions: [
        {
          type: "read",
          command: "cat README.md",
          name: "README.md",
          path: "/workspace/README.md",
        },
        {
          type: "search",
          command: "rg install",
          query: "install",
          path: "README.md",
        },
      ],
    }),
    [
      { label: "Command", value: "cat README.md\nrg install" },
      { label: "Action", value: "Read README.md\nSearch for install in README.md" },
    ],
  );
});

test("summarizes an unclassified parsed action without the shell wrapper", () => {
  assert.deepEqual(
    approvalSummaryFields({
      command: "/bin/bash -lc 'npm test'",
      commandActions: [{ type: "unknown", command: "npm test" }],
    }),
    [
      { label: "Command", value: "npm test" },
      { label: "Action", value: "Run command" },
    ],
  );
});

test("falls back to top-level fields when command actions are unavailable", () => {
  assert.deepEqual(
    approvalSummaryFields({
      command: "npm test",
      action: "run tests",
      commandActions: [null, "invalid", { command: "  " }],
    }),
    [
      { label: "Command", value: "npm test" },
      { label: "Action", value: "run tests" },
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

test("includes the working directory when the request carries one", () => {
  assert.deepEqual(
    approvalSummaryFields({
      threadId: "01a06c52-5c04-7091-b209-0c294712e68a",
      reason: "May I refresh apt metadata so I can install Go?",
      command: "/bin/bash -lc 'apt-get update'",
      cwd: "/workspace/remote.futrx",
    }),
    [
      { label: "Command", value: "/bin/bash -lc 'apt-get update'" },
      { label: "Reason", value: "May I refresh apt metadata so I can install Go?" },
      { label: "Directory", value: "/workspace/remote.futrx" },
    ],
  );
});

test("returns no fields when the payload has no supported summary keys", () => {
  assert.deepEqual(approvalSummaryFields({ input: "raw" }), []);
});
