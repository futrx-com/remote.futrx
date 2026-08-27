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
