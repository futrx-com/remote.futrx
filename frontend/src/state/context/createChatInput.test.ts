import assert from "node:assert/strict";
import test from "node:test";
import type { ChatSettings } from "../../models/settings.ts";
import { createChatInput } from "./createChatInput.ts";

const settings: ChatSettings = {
  provider: "codex",
  model: "gpt-fast",
  mode: "default",
  reasoningEffort: "medium",
  serviceTier: "priority",
};

test("carries the saved chat preferences onto the payload", () => {
  assert.deepEqual(createChatInput(settings, "project-1"), {
    provider: "codex",
    model: "gpt-fast",
    mode: "default",
    reasoningEffort: "medium",
    serviceTier: "priority",
    projectId: "project-1",
  });
});

test("omits projectId entirely for a loose chat", () => {
  const input = createChatInput(settings);
  // Absent, not undefined: the two serialize differently.
  assert.ok(!("projectId" in input));
  assert.equal(JSON.stringify(input).includes("projectId"), false);
});

test("treats an empty project id as no project", () => {
  assert.ok(!("projectId" in createChatInput(settings, "")));
});
