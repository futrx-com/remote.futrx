import assert from "node:assert/strict";
import test from "node:test";
import { chatFindSenderService } from "./chatFindSenderService.ts";

test("find results tell the user's messages apart from the agent's", () => {
  assert.deepEqual(chatFindSenderService.describe("user"), { role: "user", label: "You" });
  assert.deepEqual(chatFindSenderService.describe("assistant_text"), { role: "agent", label: "Assistant" });
  assert.deepEqual(chatFindSenderService.describe("thinking"), { role: "agent", label: "Assistant · thinking" });
});

test("find results label tool activity and errors", () => {
  assert.deepEqual(chatFindSenderService.describe("tool_use_start"), { role: "tool", label: "Tool call" });
  assert.deepEqual(chatFindSenderService.describe("tool_use_end"), { role: "tool", label: "Tool output" });
  assert.deepEqual(chatFindSenderService.describe("error"), { role: "error", label: "Error" });
});

test("find results attribute unknown event types to the agent", () => {
  assert.deepEqual(chatFindSenderService.describe("collaboration"), { role: "agent", label: "Assistant" });
});
