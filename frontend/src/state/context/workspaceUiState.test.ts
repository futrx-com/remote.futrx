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
  });

  const modalOpen = workspaceUiState.reduce(open, { type: "open-create-project" });
  assert.equal(modalOpen.createProjectOpen, true);
  assert.equal(
    workspaceUiState.reduce(modalOpen, { type: "close-create-project" }).createProjectOpen,
    false
  );
});
