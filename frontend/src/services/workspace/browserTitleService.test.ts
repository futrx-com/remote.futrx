import assert from "node:assert/strict";
import test from "node:test";
import type { ChatMeta } from "../../models/chat.ts";
import { browserTitleForWorkspace } from "./browserTitleService.ts";

const chats = [
  { id: "open", lastMessageAt: 10, lastReadAt: 1 },
  { id: "other", lastMessageAt: 12, lastReadAt: 2 },
  { id: "read", lastMessageAt: 8, lastReadAt: 8 },
  { id: "running", lastMessageAt: 15, lastReadAt: 0, running: true },
] as ChatMeta[];

test("the title counts unread finished chats away from the focused chat", () => {
  assert.equal(browserTitleForWorkspace(chats, "open", "chat", true), "(1) remote.futrx");
  assert.equal(browserTitleForWorkspace(chats, "open", "chat", false), "(2) remote.futrx");
  assert.equal(browserTitleForWorkspace(chats, "open", "settings", true), "(2) remote.futrx");
});

test("the title returns to normal after notifications are read", () => {
  assert.equal(browserTitleForWorkspace(chats.slice(2, 3), null, "chat", false), "remote.futrx");
});
