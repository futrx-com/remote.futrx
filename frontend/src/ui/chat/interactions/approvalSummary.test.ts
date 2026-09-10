import assert from "node:assert/strict";
import test from "node:test";
import { summarizeApprovalRequest } from "./approvalSummary.ts";

test("summarizes a command approval with reason, command and cwd", () => {
  assert.deepEqual(
    summarizeApprovalRequest({
      kind: "command",
      threadId: "01a06c52-5c04-7091-b209-0c294712e68a",
      reason: "May I refresh apt metadata so I can install Go?",
      command: "/bin/bash -lc 'apt-get update'",
      cwd: "/workspace/remote.futrx",
    }),
    {
      reason: "May I refresh apt metadata so I can install Go?",
      command: "/bin/bash -lc 'apt-get update'",
      cwd: "/workspace/remote.futrx",
    }
  );
});

test("ignores machine-only payloads without user-facing fields", () => {
  assert.deepEqual(
    summarizeApprovalRequest({ threadId: "abc", turnId: "def", startedAtMs: 17852399269 }),
    {}
  );
});

test("drops blank strings", () => {
  assert.deepEqual(summarizeApprovalRequest({ reason: "  ", command: "", cwd: " " }), {});
});
