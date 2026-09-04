import assert from "node:assert/strict";
import test from "node:test";
import type { ChatEvent } from "../../../models/chat.ts";
import { chatEventStateProjector } from "./chatEventStateProjector.ts";

test("projects chat events into the existing message and usage model", () => {
  const events: ChatEvent[] = [
    { type: "user", text: "hello", t: 1 },
    { type: "assistant_text", text: "hel", t: 2 },
    { type: "assistant_text", text: "lo", t: 3 },
    { type: "tool_use_start", id: "tool-1", name: "shell", input: { command: "pwd" }, t: 4 },
    { type: "tool_use_end", id: "tool-1", output: "/workspace", isError: false, t: 5 },
    { type: "complete", usage: { input_tokens: 3, output_tokens: 5 }, t: 6 },
  ];

  const state = chatEventStateProjector.fromEvents(events, {
    hasMore: false,
  });

  assert.deepEqual(state.blocks, [
    { type: "user", text: "hello", t: 1 },
    {
      type: "assistant",
      parts: [
        { kind: "text", text: "hello" },
        {
          kind: "tool",
          id: "tool-1",
          name: "shell",
          input: { command: "pwd" },
          output: "/workspace",
          isError: false,
          status: "done",
        },
      ],
      t: 2,
      isComplete: true,
    },
  ]);

  assert.deepEqual(state.usageTotals, {
    inputTokens: 3,
    outputTokens: 5,
    cacheReadTokens: 0,
    cacheWriteTokens: 0,
  });
});

test("coalesces streamed reasoning deltas into one message part", () => {
  const events: ChatEvent[] = [
    { type: "user", text: "inspect it", t: 1 },
    { type: "thinking", text: "Playwright ", t: 2 },
    { type: "thinking", text: "isn't installed. ", t: 3 },
    { type: "thinking", text: "I'll use the browser instead.", t: 4 },
    { type: "assistant_text", text: "I opened the page.", t: 5 },
    { type: "complete", t: 6 },
  ];

  const state = chatEventStateProjector.fromEvents(events, { hasMore: false });

  assert.deepEqual(state.blocks, [
    { type: "user", text: "inspect it", t: 1 },
    {
      type: "assistant",
      parts: [
        {
          kind: "thinking",
          text: "Playwright isn't installed. I'll use the browser instead.",
        },
        { kind: "text", text: "I opened the page." },
      ],
      t: 2,
      isComplete: true,
    },
  ]);
});

test("preserves pending interactions and final subagent reports across replay", () => {
  const events: ChatEvent[] = [
    { type: "user", text: "delegate", t: 1 },
    {
      type: "interaction_request",
      id: `"approval-1"`,
      name: "item/commandExecution/requestApproval",
      input: { command: "npm test" },
      status: "approval",
      t: 2,
    },
    { type: "interaction_resolved", id: `"approval-1"`, status: "answered", t: 3 },
    {
      type: "collaboration",
      id: "collab-1",
      name: "spawn_agent",
      status: "completed",
      data: {
        receiverThreadIds: ["child-1"],
        agentsStates: { child: { status: "completed", message: "child report" } },
      },
      t: 4,
    },
  ];

  const state = chatEventStateProjector.fromEvents(events, { hasMore: false });
  const assistant = state.blocks[1];
  assert.equal(assistant.type, "assistant");
  if (assistant.type !== "assistant") return;
  assert.deepEqual(assistant.parts[0], {
    kind: "interaction",
    id: `"approval-1"`,
    method: "item/commandExecution/requestApproval",
    input: { command: "npm test" },
    interactionKind: "approval",
    supportsCancellation: true,
    status: "answered",
  });
  assert.deepEqual(assistant.parts[1], {
    kind: "collaboration",
    id: "collab-1",
    name: "spawn_agent",
    status: "completed",
    data: {
      receiverThreadIds: ["child-1"],
      agentsStates: { child: { status: "completed", message: "child report" } },
    },
  });
});

test("closes an unfinished collaboration when its parent turn completes", () => {
  const events: ChatEvent[] = [
    { type: "user", text: "delegate", t: 1 },
    {
      type: "collaboration",
      id: "wait-1",
      name: "spawn_agent",
      status: "inProgress",
      data: {},
      t: 2,
    },
    { type: "complete", t: 3 },
  ];

  const state = chatEventStateProjector.fromEvents(events, { hasMore: false });
  const assistant = state.blocks[1];
  assert.equal(assistant.type, "assistant");
  if (assistant.type !== "assistant") return;
  assert.deepEqual(assistant.parts, [{
    kind: "collaboration",
    id: "wait-1",
    name: "spawn_agent",
    status: "turnEnded",
    data: {},
  }]);
  assert.equal(assistant.isComplete, true);
});

test("replaces child-thread activity with the completed subagent report", () => {
  const events: ChatEvent[] = [
    { type: "user", text: "delegate", t: 1 },
    {
      type: "collaboration",
      id: "subagent:child-1",
      name: "Subagent",
      status: "inProgress",
      data: {
        type: "subagentThread",
        receiverThreadIds: ["child-1"],
        agentsStates: { "child-1": { status: "inProgress" } },
        toolCount: 1,
      },
      t: 2,
    },
    {
      type: "collaboration",
      id: "subagent:child-1",
      name: "Subagent",
      status: "completed",
      data: {
        type: "subagentThread",
        receiverThreadIds: ["child-1"],
        agentsStates: { "child-1": { status: "completed", message: "child report" } },
        toolCount: 2,
      },
      t: 3,
    },
    { type: "assistant_text", text: "parent synthesis", t: 4 },
    { type: "complete", t: 5 },
  ];

  const state = chatEventStateProjector.fromEvents(events, { hasMore: false });
  const assistant = state.blocks[1];
  assert.equal(assistant.type, "assistant");
  if (assistant.type !== "assistant") return;
  assert.equal(assistant.parts.length, 2);
  assert.deepEqual(assistant.parts[0], {
    kind: "collaboration",
    id: "subagent:child-1",
    name: "Subagent",
    status: "completed",
    data: {
      type: "subagentThread",
      receiverThreadIds: ["child-1"],
      agentsStates: { "child-1": { status: "completed", message: "child report" } },
      toolCount: 2,
    },
  });
  assert.deepEqual(assistant.parts[1], { kind: "text", text: "parent synthesis" });
});

test("does not render legacy empty wait mechanics", () => {
  const events: ChatEvent[] = [
    { type: "user", text: "delegate", t: 1 },
    {
      type: "collaboration",
      id: "wait-1",
      name: "wait",
      status: "completed",
      data: { receiverThreadIds: [], agentsStates: {} },
      t: 2,
    },
    { type: "assistant_text", text: "parent result", t: 3 },
    { type: "complete", t: 4 },
  ];

  const state = chatEventStateProjector.fromEvents(events, { hasMore: false });
  const assistant = state.blocks[1];
  assert.equal(assistant.type, "assistant");
  if (assistant.type !== "assistant") return;
  assert.deepEqual(assistant.parts, [{ kind: "text", text: "parent result" }]);
});

test("closes a running tool when its parent turn is interrupted", () => {
  const events: ChatEvent[] = [
    { type: "user", text: "run a long command", t: 1 },
    {
      type: "tool_use_start",
      id: "tool-1",
      name: "Bash",
      input: { command: "sleep 60" },
      t: 2,
    },
    { type: "turn_status", provider: "minimax", status: "interrupted", t: 3 },
  ];

  const state = chatEventStateProjector.fromEvents(events, { hasMore: false });
  const assistant = state.blocks[1];
  assert.equal(assistant.type, "assistant");
  if (assistant.type !== "assistant") return;
  assert.deepEqual(assistant.parts, [
    {
      kind: "tool",
      id: "tool-1",
      name: "Bash",
      input: { command: "sleep 60" },
      status: "done",
    },
    { kind: "turn-status", status: "interrupted", data: undefined, provider: "minimax" },
  ]);
  assert.equal(assistant.isComplete, true);
});

test("terminal turn status does not reactivate a run after its final sync", () => {
  const interrupted: ChatEvent = { type: "turn_status", status: "interrupted", t: 1 };

  assert.equal(chatEventStateProjector.statusAfter(interrupted, "ready"), "ready");
  assert.equal(chatEventStateProjector.statusAfter(interrupted, "streaming"), "streaming");
});

test("retains provider events without rendering them in the transcript", () => {
  const events: ChatEvent[] = [
    { type: "user", text: "hello", t: 1 },
    {
      type: "provider_event",
      name: "thread/settings/updated",
      data: { model: "gpt-5.6-sol" },
      t: 2,
    },
    { type: "assistant_text", text: "hello", t: 3 },
    { type: "complete", t: 4 },
  ];

  const state = chatEventStateProjector.fromEvents(events, { hasMore: false });

  assert.equal(state.events, events);
  assert.deepEqual(state.blocks, [
    { type: "user", text: "hello", t: 1 },
    {
      type: "assistant",
      parts: [{ kind: "text", text: "hello" }],
      t: 3,
      isComplete: true,
    },
  ]);
});

test("prepends an older event page before current blocks and adopts hasMore", () => {
  const latest = chatEventStateProjector.fromEvents(
    [
      { seq: 4, type: "user", text: "new question", t: 4 },
      { seq: 5, type: "assistant_text", text: "a complete long answer", t: 5 },
      { seq: 305, type: "complete", t: 305 },
    ],
    { hasMore: true, nextBefore: 4 }
  );

  const state = chatEventStateProjector.prepend(latest, {
    events: [
      { seq: 1, type: "user", text: "older question", t: 1 },
      { seq: 2, type: "assistant_text", text: "older answer", t: 2 },
      { seq: 3, type: "complete", t: 3 },
    ],
    hasMore: false,
    lastSeq: 305,
  });

  assert.deepEqual(state.blocks, [
    { type: "user", text: "older question", t: 1 },
    {
      type: "assistant",
      parts: [{ kind: "text", text: "older answer" }],
      t: 2,
      isComplete: true,
    },
    { type: "user", text: "new question", t: 4 },
    {
      type: "assistant",
      parts: [{ kind: "text", text: "a complete long answer" }],
      t: 5,
      isComplete: true,
    },
  ]);
  assert.equal(state.hasOlder, false);
});
