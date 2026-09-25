import assert from "node:assert/strict";
import test from "node:test";
import { parseWorkspaceRoute, workspaceRoutePath } from "./workspaceNavigation.ts";

test("chat and settings URLs restore the intended view", () => {
  assert.deepEqual(parseWorkspaceRoute("/chats/abcdef12"), {
    view: "chat", chatId: "abcdef12", tab: "appearance",
  });
  assert.deepEqual(parseWorkspaceRoute("/settings/notifications"), {
    view: "settings", chatId: null, tab: "notifications",
  });
  assert.deepEqual(parseWorkspaceRoute("/settings/unknown"), {
    view: "settings", chatId: null, tab: "appearance",
  });
  assert.equal(workspaceRoutePath(parseWorkspaceRoute("/settings")), "/settings");
  assert.equal(workspaceRoutePath(parseWorkspaceRoute("/chats/abcdef12")), "/chats/abcdef12");
});

test("legacy notification query selects the chat", () => {
  assert.equal(parseWorkspaceRoute("/", "?chat=abcdef12").chatId, "abcdef12");
  assert.equal(parseWorkspaceRoute("/", "?chat=invalid").chatId, null);
});
