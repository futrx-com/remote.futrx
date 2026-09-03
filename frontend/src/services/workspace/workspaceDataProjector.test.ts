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
