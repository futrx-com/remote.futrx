import assert from "node:assert/strict";
import test from "node:test";
import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import { workspaceSidebarState } from "./workspaceSidebarState.ts";
import { workspaceUiState } from "./workspaceUiState.ts";

const projects: ProjectMeta[] = [
  {
    id: "older",
    name: "Older project",
    slug: "older-project",
    cwd: "/older",
    containerName: "older",
    status: "running",
    order: 1,
    createdAt: 1,
    updatedAt: 1,
  },
  {
    id: "newer",
    name: "Newer project",
    slug: "newer-project",
    cwd: "/newer",
    containerName: "newer",
    status: "running",
    order: 2,
    createdAt: 2,
    updatedAt: 2,
  },
];

const chats: ChatMeta[] = [
  { id: "old-chat", title: "Old", projectId: "newer", createdAt: 1, lastMessageAt: 1 },
  { id: "new-chat", title: "New", projectId: "newer", createdAt: 2, lastMessageAt: 2 },
  { id: "loose", title: "Loose", createdAt: 3, lastMessageAt: 3 },
];

test("preserves workspace UI transitions and sidebar ordering", () => {
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

  const model = workspaceSidebarState.model(chats, projects, "");
  assert.deepEqual(model.visibleProjects.map((node) => node.project.id), ["newer", "older"]);
  assert.deepEqual(model.visibleProjects[0].chats.map((chat) => chat.id), ["new-chat", "old-chat"]);
  assert.deepEqual(model.visibleLooseChats.map((chat) => chat.id), ["loose"]);
});
