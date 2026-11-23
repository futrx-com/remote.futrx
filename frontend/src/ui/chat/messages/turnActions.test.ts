import assert from "node:assert/strict";
import test from "node:test";
import type { AssistantMessageBlock, ChatMessageBlock } from "../../../models/chatMessage";
import { assistantText, assistantTurnActions } from "./turnActions.ts";

function reply(t: number, texts: string[], isComplete = true): AssistantMessageBlock {
  return { type: "assistant", t, isComplete, parts: texts.map((text) => ({ kind: "text" as const, text })) };
}

const thread: ChatMessageBlock[] = [
  { type: "user", t: 100, text: "first question" },
  reply(110, ["first answer"]),
  { type: "user", t: 200, text: "second question" },
  reply(210, ["second answer"]),
];

test("an earlier turn retries its own prompt and branches before the next one", () => {
  assert.deepEqual(assistantTurnActions(thread, 1, false), {
    text: "first answer",
    prompt: { t: 100, text: "first question" },
    branchBeforeT: 200,
  });
});

test("the last turn branches the whole chat", () => {
  const actions = assistantTurnActions(thread, 3, false);
  assert.equal(actions?.branchBeforeT, undefined);
  assert.deepEqual(actions?.prompt, { t: 200, text: "second question" });
});

test("no actions while the reply is streaming or incomplete", () => {
  assert.equal(assistantTurnActions(thread, 3, true), null);
  const unfinished = [...thread.slice(0, 3), reply(210, ["partial"], false)];
  assert.equal(assistantTurnActions(unfinished, 3, false), null);
});

test("a turn split over several assistant blocks gets one row, on its last block", () => {
  const split: ChatMessageBlock[] = [
    { type: "user", t: 100, text: "question" },
    reply(110, ["part one"]),
    reply(120, ["part two"]),
  ];
  assert.equal(assistantTurnActions(split, 1, false), null);
  assert.equal(assistantTurnActions(split, 2, false)?.prompt?.t, 100);
});

test("user and error blocks, and replies without text, get no actions", () => {
  assert.equal(assistantTurnActions(thread, 0, false), null);
  const toolsOnly: ChatMessageBlock[] = [
    { type: "user", t: 100, text: "question" },
    { type: "assistant", t: 110, isComplete: true, parts: [{ kind: "turn-status", status: "failed" }] },
  ];
  assert.equal(assistantTurnActions(toolsOnly, 1, false), null);
});

test("a reply without a prompt before it can still be copied and branched", () => {
  const loaded: ChatMessageBlock[] = [reply(110, ["older answer"]), { type: "user", t: 200, text: "next" }];
  assert.deepEqual(assistantTurnActions(loaded, 0, false), { text: "older answer", prompt: undefined, branchBeforeT: 200 });
});

test("copied text joins the text parts and skips tools and thinking", () => {
  const block: AssistantMessageBlock = {
    type: "assistant",
    t: 1,
    isComplete: true,
    parts: [
      { kind: "thinking", text: "hidden" },
      { kind: "text", text: "Run this:\n" },
      { kind: "tool", id: "1", name: "Bash", input: {}, status: "done" },
      { kind: "text", text: "```sh\nls\n```" },
    ],
  };
  assert.equal(assistantText(block), "Run this:\n\n```sh\nls\n```");
});
