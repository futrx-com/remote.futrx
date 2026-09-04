import assert from "node:assert/strict";
import test from "node:test";
import type { UserSettings } from "../../models/settings.ts";
import { createChatInput } from "./createChatInput.ts";

const settings: Pick<UserSettings, "chat" | "projectChat"> = {
  chat: {
    provider: "kimi",
    model: "kimi-for-host",
    mode: "default",
    reasoningEffort: "medium",
    serviceTier: "priority",
    approvalPolicy: "on-request",
    sandboxPolicy: "workspaceWrite",
  },
  projectChat: {
    provider: "minimax",
    model: "MiniMax-M2.7",
    mode: "default",
    reasoningEffort: "",
    serviceTier: "",
    approvalPolicy: "never",
    sandboxPolicy: "readOnly",
  },
};

test("uses project chat preferences for a project chat", () => {
  assert.deepEqual(createChatInput(settings, "project-1"), {
    provider: "minimax",
    model: "MiniMax-M2.7",
    mode: "default",
    reasoningEffort: "",
    serviceTier: "",
    approvalPolicy: "never",
    sandboxPolicy: "readOnly",
    projectId: "project-1",
  });
});

test("uses host chat preferences and omits projectId for a loose chat", () => {
  const input = createChatInput(settings);
  assert.equal(input.provider, "kimi");
  assert.equal(input.model, "kimi-for-host");
  assert.equal(input.approvalPolicy, "on-request");
  assert.equal(input.sandboxPolicy, "workspaceWrite");
  // Absent, not undefined: the two serialize differently.
  assert.ok(!("projectId" in input));
  assert.equal(JSON.stringify(input).includes("projectId"), false);
});

test("treats an empty project id as no project", () => {
  assert.ok(!("projectId" in createChatInput(settings, "")));
});
