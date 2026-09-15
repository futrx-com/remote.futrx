import assert from "node:assert/strict";
import test from "node:test";
import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import { workspaceDataProjector } from "./workspaceDataProjector.ts";
import { workspaceSidebarService } from "./workspaceSidebarService.ts";

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

test("orders projects and their chats by recency", () => {
  const model = workspaceSidebarService.model(chats, projects);
  assert.deepEqual(model.visibleProjects.map((node) => node.project.id), ["newer", "older"]);
  assert.deepEqual(model.visibleProjects[0].chats.map((chat) => chat.id), ["new-chat", "old-chat"]);
  assert.deepEqual(model.visibleLooseChats.map((chat) => chat.id), ["loose"]);
});

test("a chat left pointing at a deleted project stays visible as unassigned", () => {
  const orphaned: ChatMeta[] = [
    ...chats,
    { id: "orphan", title: "Orphan", projectId: "deleted", createdAt: 4, lastMessageAt: 4 },
  ];
  const model = workspaceSidebarService.model(orphaned, projects);
  // Bucketed under a project that is never rendered, it would vanish entirely.
  assert.deepEqual(model.visibleLooseChats.map((chat) => chat.id), ["orphan", "loose"]);
  assert.equal(model.visibleProjects.every((node) => node.project.id !== "deleted"), true);
});

test("a deleted active chat hands over to the next chat, not the empty state", () => {
  const remaining = chats.filter((chat) => chat.id !== "new-chat");
  assert.equal(workspaceSidebarService.isActiveChatMissing(remaining, "new-chat"), true);
  assert.equal(workspaceSidebarService.replacementChatId(remaining), "old-chat");
  // Same pick a fresh load would make, so the handover is not a special case.
  assert.equal(workspaceSidebarService.initialChatId(true, null, remaining), "old-chat");
  // Deleting the last chat is the one case that legitimately clears selection.
  assert.equal(workspaceSidebarService.replacementChatId([]), null);
});

test("a chat created from this client opens instead of bouncing back", () => {
  // The create response lands before the chat.upsert that carries the new chat
  // into the list, so the selection is made against a list that predates it.
  const created: ChatMeta = {
    id: "created",
    title: "New chat",
    projectId: "newer",
    createdAt: 4,
    lastMessageAt: 4,
  };

  // Selecting it against the stale list reads as "the active chat is gone", and
  // the delete handover then drags the user back to the chat they came from.
  const listed = workspaceDataProjector.replaceChats(chats, []);
  assert.equal(workspaceSidebarService.isActiveChatMissing(listed, created.id), true);
  assert.equal(workspaceSidebarService.replacementChatId(listed), "loose");

  // Seeding the created chat locally closes that window: it is already in the
  // list by the time the handover check runs, so the selection stands.
  const seeded = workspaceDataProjector.upsertChat(listed, created);
  assert.equal(workspaceSidebarService.isActiveChatMissing(seeded, created.id), false);
  assert.equal(workspaceSidebarService.activeChat(seeded, created.id)?.id, created.id);
  // Newest first, so the new chat is also the one the sidebar shows on top.
  assert.equal(seeded[0].id, created.id);

  // The chat.upsert that follows is a no-op rather than a second insertion.
  assert.equal(workspaceDataProjector.upsertChat(seeded, created), seeded);
});

test("project drag-reorder respects which side of the target it was dropped on", () => {
  const ids = ["a", "b", "c"];
  const reorder = workspaceSidebarService.reorderProjectIds.bind(workspaceSidebarService);

  // Dropping past the last project must land last. Splicing at the target's
  // pre-removal index used to leave it one slot short, at ["b", "a", "c"].
  assert.deepEqual(reorder(ids, "a", "c", "after"), ["b", "c", "a"]);
  assert.deepEqual(reorder(ids, "a", "c", "before"), ["b", "a", "c"]);
  assert.deepEqual(reorder(ids, "c", "a", "before"), ["c", "a", "b"]);
  assert.deepEqual(reorder(ids, "c", "a", "after"), ["a", "c", "b"]);

  // Drops that change nothing report null so no reorder request is sent.
  assert.equal(reorder(ids, "a", "a", "before"), null);
  assert.equal(reorder(ids, "a", "b", "before"), null);
  assert.equal(reorder(ids, "b", "a", "after"), null);
  assert.equal(reorder(ids, "a", "missing", "after"), null);
});
