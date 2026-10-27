import assert from "node:assert/strict";
import test from "node:test";
import type { MediaViewerItem } from "../../../models/files.ts";
import { mediaViewerStore } from "./mediaViewerStore.ts";

test.afterEach(() => {
  mediaViewerStore.getState().close();
});

test("opens and closes the selected media item", () => {
  const item: MediaViewerItem = {
    url: "/api/chats/chat-1/media?path=image.png",
    name: "image.png",
    kind: "image",
  };
  const seen: Array<MediaViewerItem | null> = [];
  const unsubscribe = mediaViewerStore.subscribe((state) => {
    seen.push(state.item);
  });

  mediaViewerStore.getState().open(item);
  assert.equal(mediaViewerStore.getState().item, item);

  mediaViewerStore.getState().close();
  assert.equal(mediaViewerStore.getState().item, null);
  assert.deepEqual(seen, [item, null]);

  unsubscribe();
});

test("closing an empty viewer does not publish a redundant update", () => {
  mediaViewerStore.getState().close();
  const before = mediaViewerStore.getState();
  let notifications = 0;
  const unsubscribe = mediaViewerStore.subscribe(() => {
    notifications += 1;
  });

  mediaViewerStore.getState().close();

  assert.equal(mediaViewerStore.getState(), before);
  assert.equal(notifications, 0);
  unsubscribe();
});
