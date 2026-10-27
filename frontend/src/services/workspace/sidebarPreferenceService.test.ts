import assert from "node:assert/strict";
import test from "node:test";
import type { ChatMeta } from "../../models/chat.ts";
import type { ProjectMeta } from "../../models/project.ts";
import { STORAGE_KEYS } from "../../config/storageKeys.ts";
import { sidebarPreferenceService } from "./sidebarPreferenceService.ts";

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

test("an expanded project stays expanded after a reload", () => {
  const store = new Map<string, string>();
  (globalThis as { localStorage?: unknown }).localStorage = {
    getItem: (key: string) => store.get(key) ?? null,
    setItem: (key: string, value: string) => void store.set(key, value),
  };

  // "newer" has unread chats, "older" does not: the first-sight seeding folds
  // "older" and leaves "newer" open.
  const seeded = sidebarPreferenceService.seedCollapsedProjects(projects, chats, {});
  assert.deepEqual(seeded, { older: true, newer: false });

  // The user expands "older" and folds "newer"; both choices are written out.
  const chosen = { ...seeded, older: false, newer: true };
  sidebarPreferenceService.writeCollapsedProjects(chosen);

  // A reload starts from what was stored, and seeding leaves those entries be.
  const restored = sidebarPreferenceService.readCollapsedProjects();
  assert.deepEqual(restored, chosen);
  assert.deepEqual(
    sidebarPreferenceService.seedCollapsedProjects(projects, chats, restored),
    chosen
  );

  // Junk in storage falls back to seeding rather than breaking the sidebar.
  store.set(STORAGE_KEYS.collapsedProjects, "not json");
  assert.deepEqual(sidebarPreferenceService.readCollapsedProjects(), {});
  store.set(STORAGE_KEYS.collapsedProjects, '{"older":"yes"}');
  assert.deepEqual(sidebarPreferenceService.readCollapsedProjects(), {});
});
