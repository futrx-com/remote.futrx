import assert from "node:assert/strict";
import test from "node:test";
import type { ChatMessageBlock } from "../../../models/chatMessage.ts";
import { hasVisibleAssistantContent, showTerminalTurnStatus, showTurnActivity } from "./turnActivity.ts";

const completed: ChatMessageBlock = { type: "assistant", parts: [], t: 1, isComplete: true };
const running: ChatMessageBlock = { type: "assistant", parts: [
  { kind: "turn-status", status: "inProgress" },
], t: 2, isComplete: false };

test("shows turn activity immediately after a local send, before the server echoes it", () => {
  assert.equal(showTurnActivity("streaming", [completed], true), true);
  assert.equal(showTurnActivity("streaming", [completed, { type: "user", text: "hi", t: 2 }], false), true);
  assert.equal(showTurnActivity("streaming", [completed, running], false), true);
});

test("stops activity when the turn ends or a visible thinking part takes over", () => {
  assert.equal(showTurnActivity("streaming", [completed], false), false);
  assert.equal(showTurnActivity("ready", [running], true), false);
  assert.equal(showTurnActivity("streaming", [{ ...running, parts: [{ kind: "thinking", text: "reasoning" }] }], false), false);
  assert.equal(showTurnActivity("streaming", [{ type: "error", message: "failed", t: 3 }], false), false);
});

test("hides routine and interrupted provider status rows while retaining failures", () => {
  assert.equal(hasVisibleAssistantContent(running), false);
  assert.equal(showTerminalTurnStatus("completed"), false);
  assert.equal(showTerminalTurnStatus("interrupted"), false);
  assert.equal(showTerminalTurnStatus("failed"), true);
  assert.equal(hasVisibleAssistantContent({ ...running, parts: [{ kind: "turn-status", status: "interrupted" }] }), false);
  assert.equal(hasVisibleAssistantContent({ ...running, parts: [{ kind: "turn-status", status: "failed" }] }), true);
});
