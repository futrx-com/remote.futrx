import assert from "node:assert/strict";
import test from "node:test";
import { chatMessageBlockBuilder } from "./chatMessageBlockBuilder.ts";

test("replayed prompts retain their individual Remote account snapshots", () => {
  const blocks = chatMessageBlockBuilder.fromEvents([
    { type: "user", text: "First", t: 1, userEmail: "alice@example.com" },
    { type: "assistant_text", text: "Reply", t: 2 },
    { type: "user", text: "Second", t: 3, userEmail: "bob@example.com" },
  ]);
  assert.deepEqual(blocks.filter(block => block.type === "user"), [
    { type: "user", text: "First", t: 1, userEmail: "alice@example.com" },
    { type: "user", text: "Second", t: 3, userEmail: "bob@example.com" },
  ]);
});

test("legacy prompts do not inherit an author from another turn", () => {
  const blocks = chatMessageBlockBuilder.fromEvents([
    { type: "user", text: "Attributed", t: 1, userEmail: "alice@example.com" },
    { type: "user", text: "Legacy", t: 2 },
  ]);
  assert.deepEqual(blocks[1], { type: "user", text: "Legacy", t: 2 });
});
