import assert from "node:assert/strict";
import test from "node:test";
import type { ChatMeta } from "../../models/chat.ts";
import { workspaceDataProjector } from "./workspaceDataProjector.ts";

test("detects generic and legacy provider session changes", () => {
  const current: ChatMeta[] = [{
    id: "chat",
    title: "Chat",
    createdAt: 1,
    lastMessageAt: 1,
    sessions: { "future-agent": "session-1" },
    kimiSessionId: "kimi-1",
    antigravitySessionId: "agy-1",
  }];

  const same = workspaceDataProjector.replaceChats([{
    ...current[0],
    sessions: { "future-agent": "session-1" },
  }], current);
  assert.equal(same, current);

  const genericChanged = workspaceDataProjector.replaceChats([{
    ...current[0],
    sessions: { "future-agent": "session-2" },
  }], current);
  assert.notEqual(genericChanged, current);

  const legacyChanged = workspaceDataProjector.replaceChats([{
    ...current[0],
    antigravitySessionId: "agy-2",
  }], current);
  assert.notEqual(legacyChanged, current);
});

test("detects selected-skill removal from a workspace upsert", () => {
  const current: ChatMeta[] = [{
    id: "chat",
    title: "Chat",
    createdAt: 1,
    lastMessageAt: 1,
    selectedSkills: [{
      name: "Code Refactorer",
      command: "code-refactorer",
      provider: "codex",
      source: "project",
    }],
  }];
  const { selectedSkills: _selectedSkills, ...withoutSelectedSkills } = current[0];

  const removed = workspaceDataProjector.upsertChat(current, withoutSelectedSkills);

  assert.notEqual(removed, current);
  assert.equal(removed[0].selectedSkills, undefined);
});

test("treats omitted and empty selected-skill collections as equivalent", () => {
  const current: ChatMeta[] = [{
    id: "chat",
    title: "Chat",
    createdAt: 1,
    lastMessageAt: 1,
  }];

  const same = workspaceDataProjector.upsertChat(current, {
    ...current[0],
    selectedSkills: [],
  });

  assert.equal(same, current);
});

test("detects selected-skill field changes from a workspace upsert", () => {
  const current: ChatMeta[] = [{
    id: "chat",
    title: "Chat",
    createdAt: 1,
    lastMessageAt: 1,
    selectedSkills: [{ name: "browser", command: "browser", provider: "claude", source: "builtin" }],
  }];

  const same = workspaceDataProjector.upsertChat(current, {
    ...current[0],
    selectedSkills: [{ name: "browser", command: "browser", provider: "claude", source: "builtin" }],
  });
  assert.equal(same, current);

  // The chip renders `name || command`, so a rename under a stable command has
  // to reach the list or the old label stays on screen.
  const renamed = workspaceDataProjector.upsertChat(current, {
    ...current[0],
    selectedSkills: [{ name: "Browser", command: "browser", provider: "claude", source: "builtin" }],
  });
  assert.notEqual(renamed, current);
  assert.equal(renamed[0].selectedSkills?.[0].name, "Browser");

  const swapped = workspaceDataProjector.upsertChat(current, {
    ...current[0],
    selectedSkills: [{ name: "run", command: "run", provider: "claude", source: "builtin" }],
  });
  assert.notEqual(swapped, current);
  assert.equal(swapped[0].selectedSkills?.[0].command, "run");
});

test("detects approval and sandbox preference changes from a workspace upsert", () => {
  const current: ChatMeta[] = [{
    id: "chat",
    title: "Chat",
    createdAt: 1,
    lastMessageAt: 1,
    approvalPolicy: "on-request",
    sandboxPolicy: "workspaceWrite",
  }];

  const approvalChanged = workspaceDataProjector.upsertChat(current, {
    ...current[0],
    approvalPolicy: "never",
  });
  assert.notEqual(approvalChanged, current);
  assert.equal(approvalChanged[0].approvalPolicy, "never");

  const sandboxChanged = workspaceDataProjector.upsertChat(current, {
    ...current[0],
    sandboxPolicy: "dangerFullAccess",
  });
  assert.notEqual(sandboxChanged, current);
  assert.equal(sandboxChanged[0].sandboxPolicy, "dangerFullAccess");
});
