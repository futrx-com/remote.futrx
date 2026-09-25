import assert from "node:assert/strict";
import test from "node:test";
import { workspaceUiState } from "./workspaceUiState.ts";

test("preserves workspace UI transitions", () => {
  const open = workspaceUiState.reduce(workspaceUiState.createInitial(), { type: "open-sidebar" });
  assert.deepEqual(workspaceUiState.reduce(open, { type: "select-chat", chatId: "new-chat" }), {
    activeChatId: "new-chat",
    containerProjectId: null,
    sidebarOpen: false,
    createProjectOpen: false,
    view: "chat",
    settingsTab: "appearance",
  });

  const modalOpen = workspaceUiState.reduce(open, { type: "open-create-project" });
  assert.equal(modalOpen.createProjectOpen, true);
  assert.equal(
    workspaceUiState.reduce(modalOpen, { type: "close-create-project" }).createProjectOpen,
    false
  );
});

test("restores chat and selected settings tab from browser history", () => {
  const chat = workspaceUiState.reduce(workspaceUiState.createInitial("abcdef12"), {
    type: "restore-route", view: "settings", chatId: null, tab: "notifications",
  });
  assert.equal(chat.view, "settings");
  assert.equal(chat.settingsTab, "notifications");
  assert.equal(chat.activeChatId, "abcdef12");

  const restored = workspaceUiState.reduce(chat, {
    type: "restore-route", view: "chat", chatId: "deadbeef", tab: "appearance",
  });
  assert.equal(restored.view, "chat");
  assert.equal(restored.activeChatId, "deadbeef");
});
