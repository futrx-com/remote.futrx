import assert from "node:assert/strict";
import test from "node:test";
import type { ChatMeta } from "../../../models/chat.ts";
import type { WorkspaceMessage } from "../../../types/workspaceApi.ts";
import { createWorkspaceStore } from "./workspaceStore.ts";

function chat(id: string, lastMessageAt = 1): ChatMeta {
  return { id, title: id, createdAt: 1, lastMessageAt };
}

/** A store wired to a feed the test drives by hand. */
function connectedStore() {
  let push: ((message: WorkspaceMessage) => void) | null = null;
  let closed = 0;
  const store = createWorkspaceStore((onMessage) => {
    push = onMessage;
    return () => {
      closed += 1;
      push = null;
    };
  });
  store.getState().setConnected(true);
  return {
    store,
    send: (message: WorkspaceMessage) => push?.(message),
    closedCount: () => closed,
  };
}

test("nothing is loaded before the first snapshot", () => {
  const { store } = connectedStore();
  assert.deepEqual(store.getState().snapshot, { chats: [], projects: [], loaded: false });
});

test("the first snapshot marks the workspace loaded", () => {
  const { store, send } = connectedStore();
  send({ type: "workspace.snapshot", chats: [chat("a")], projects: [] });
  assert.equal(store.getState().snapshot.loaded, true);
  assert.deepEqual(store.getState().snapshot.chats.map((c) => c.id), ["a"]);
});

test("an upsert after the snapshot keeps it loaded", () => {
  const { store, send } = connectedStore();
  send({ type: "workspace.snapshot", chats: [], projects: [] });
  send({ type: "chat.upsert", chat: chat("b") });
  assert.equal(store.getState().snapshot.loaded, true);
  assert.deepEqual(store.getState().snapshot.chats.map((c) => c.id), ["b"]);
});

test("a delete removes the chat", () => {
  const { store, send } = connectedStore();
  send({ type: "workspace.snapshot", chats: [chat("a"), chat("b")], projects: [] });
  send({ type: "chat.delete", id: "a" });
  assert.deepEqual(store.getState().snapshot.chats.map((c) => c.id), ["b"]);
});

test("a message that changes nothing does not notify or replace the snapshot", () => {
  const { store, send } = connectedStore();
  send({ type: "workspace.snapshot", chats: [chat("a")], projects: [] });

  let notifications = 0;
  store.subscribe(() => { notifications += 1; });
  const before = store.getState().snapshot;

  // Same chat, same fields: the projector returns the current array, so there
  // is nothing to publish. This is what keeps subscribers from re-rendering on
  // traffic rather than on change.
  send({ type: "chat.upsert", chat: chat("a") });

  assert.equal(notifications, 0);
  assert.equal(store.getState().snapshot, before);
});

test("a real change notifies once with the new snapshot", () => {
  const { store, send } = connectedStore();
  send({ type: "workspace.snapshot", chats: [chat("a")], projects: [] });

  const seen: number[] = [];
  store.subscribe((state) => seen.push(state.snapshot.chats.length));
  send({ type: "chat.upsert", chat: chat("z") });

  assert.deepEqual(seen, [2]);
});

test("disconnecting closes the feed and clears what it delivered", () => {
  const { store, send, closedCount } = connectedStore();
  send({ type: "workspace.snapshot", chats: [chat("a")], projects: [] });

  store.getState().setConnected(false);

  assert.equal(closedCount(), 1);
  assert.deepEqual(store.getState().snapshot, { chats: [], projects: [], loaded: false });
});

test("connecting twice opens one feed, and disconnecting twice closes it once", () => {
  const { store, closedCount } = connectedStore();
  store.getState().setConnected(true);
  store.getState().setConnected(false);
  store.getState().setConnected(false);
  assert.equal(closedCount(), 1);
});
