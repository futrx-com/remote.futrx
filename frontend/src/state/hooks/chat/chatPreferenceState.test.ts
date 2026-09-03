import assert from "node:assert/strict";
import test from "node:test";
import { chatPreferenceState } from "./chatPreferenceState.ts";

test("preserves normalized skill identity and chat defaults", () => {
  const selected = [{ name: "Review", command: " /REVIEW ", provider: "codex" as const }];
  const duplicate = { name: "review", command: "/review", provider: "codex" as const };

  assert.equal(chatPreferenceState.includesSkill(selected, duplicate, "claude"), true);
  assert.deepEqual(chatPreferenceState.withoutSkill(selected, duplicate, "claude"), []);
  assert.deepEqual(
    chatPreferenceState.resolveMeta(
      { id: "chat", title: "Chat", createdAt: 1, lastMessageAt: 1 },
      null,
      {
        provider: "codex",
        model: "gpt-5",
        mode: "default",
        reasoningEffort: "high",
        serviceTier: "priority",
      }
    ),
    {
      id: "chat",
      title: "Chat",
      createdAt: 1,
      lastMessageAt: 1,
      provider: "codex",
      model: "gpt-5",
      mode: "default",
      reasoningEffort: "high",
      serviceTier: "priority",
      selectedSkills: [],
    }
  );
});

test("keeps the scheduled-tasks skill identity stable after workspace provisioning", () => {
  const selected = [{
    name: "Scheduled Tasks",
    command: "scheduled-tasks",
    provider: "codex" as const,
    source: "remote",
  }];
  const provisioned = {
    name: "Scheduled Tasks",
    command: "scheduled-tasks",
    provider: "codex" as const,
    source: "project",
  };

  assert.equal(chatPreferenceState.includesSkill(selected, provisioned, "claude"), true);
  assert.deepEqual(chatPreferenceState.withoutSkill(selected, provisioned, "claude"), []);
});

test("prefers live workspace selections from another client", () => {
  const resolved = chatPreferenceState.resolveMeta(
    {
      id: "chat",
      title: "Chat",
      createdAt: 1,
      lastMessageAt: 1,
      provider: "claude",
      model: "claude-opus-current",
      mode: "plan",
      reasoningEffort: "high",
      serviceTier: "fast",
    },
    {
      id: "chat",
      title: "Chat",
      createdAt: 1,
      lastMessageAt: 1,
      provider: "codex",
      model: "gpt-previous",
      mode: "default",
      reasoningEffort: "low",
      serviceTier: "default",
    },
    {
      provider: "codex",
      model: "",
      mode: "default",
      reasoningEffort: "",
      serviceTier: "",
    },
  );

  assert.equal(resolved.provider, "claude");
  assert.equal(resolved.model, "claude-opus-current");
  assert.equal(resolved.mode, "plan");
  assert.equal(resolved.reasoningEffort, "high");
  assert.equal(resolved.serviceTier, "fast");
});

test("does not restore stale detail skills after a live workspace removal", () => {
  const resolved = chatPreferenceState.resolveMeta(
    {
      id: "chat",
      title: "Chat",
      createdAt: 1,
      lastMessageAt: 1,
      provider: "codex",
    },
    {
      id: "chat",
      title: "Chat",
      createdAt: 1,
      lastMessageAt: 1,
      provider: "codex",
      selectedSkills: [{
        name: "Code Refactorer",
        command: "code-refactorer",
        provider: "codex",
        source: "project",
      }],
    },
    {
      provider: "codex",
      model: "",
      mode: "default",
      reasoningEffort: "",
      serviceTier: "",
    },
  );

  assert.deepEqual(resolved.selectedSkills, []);
});

test("preserves an explicit per-chat Auto selection", () => {
  const resolved = chatPreferenceState.resolveMeta(
    {
      id: "chat",
      title: "Chat",
      createdAt: 1,
      lastMessageAt: 1,
      provider: "claude",
      model: "",
      mode: "default",
      reasoningEffort: "",
      serviceTier: "",
    },
    null,
    {
      provider: "codex",
      model: "gpt-global-default",
      mode: "plan",
      reasoningEffort: "high",
      serviceTier: "fast",
    },
  );

  assert.equal(resolved.provider, "claude");
  assert.equal(resolved.model, "");
  assert.equal(resolved.mode, "default");
  assert.equal(resolved.reasoningEffort, "");
  assert.equal(resolved.serviceTier, "");
});
