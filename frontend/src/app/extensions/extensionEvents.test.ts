import assert from "node:assert/strict";
import test from "node:test";

import {
  emitExtensionEvent,
  removeExtensionEventSubscriptions,
  resetExtensionEvents,
  subscribeExtensionEvent,
} from "./extensionEvents.ts";
import type { UploadCompletedEvent } from "../../models/extension.ts";

const upload: UploadCompletedEvent = {
  chatId: "chat-1",
  projectId: "project-1",
  fileName: "shot-a8ho.png",
  directory: "/workspace/.uploads",
  path: "/workspace/.uploads/shot-a8ho.png",
  size: 233361,
  claim: () => {},
};

test("a handler receives the events it subscribed to", () => {
  resetExtensionEvents();
  const seen: UploadCompletedEvent[] = [];
  subscribeExtensionEvent("s3disk", "upload.completed", (event) => {
    seen.push(event);
  });

  emitExtensionEvent("upload.completed", upload);

  assert.deepEqual(seen, [upload]);
});

test("unsubscribing stops delivery", () => {
  resetExtensionEvents();
  let calls = 0;
  const off = subscribeExtensionEvent("s3disk", "upload.completed", () => {
    calls += 1;
  });

  emitExtensionEvent("upload.completed", upload);
  off();
  emitExtensionEvent("upload.completed", upload);

  assert.equal(calls, 1);
});

test("a throwing handler does not stop the ones after it", (t) => {
  // The emitter sits inside the upload's onSuccess, so one extension throwing
  // has to cost that extension and nothing else.
  resetExtensionEvents();
  t.mock.method(console, "error", () => {});
  let reached = false;
  subscribeExtensionEvent("broken", "upload.completed", () => {
    throw new Error("boom");
  });
  subscribeExtensionEvent("s3disk", "upload.completed", () => {
    reached = true;
  });

  assert.doesNotThrow(() => emitExtensionEvent("upload.completed", upload));
  assert.equal(reached, true);
});

test("a handler may unsubscribe itself mid-emit", () => {
  resetExtensionEvents();
  let second = 0;
  const off = subscribeExtensionEvent("s3disk", "upload.completed", () => {
    off();
  });
  subscribeExtensionEvent("other", "upload.completed", () => {
    second += 1;
  });

  emitExtensionEvent("upload.completed", upload);

  assert.equal(second, 1);
});

test("removing an image drops its subscriptions and only its own", () => {
  // An uninstalled image keeps its entry module in the page, so its handlers
  // would otherwise keep firing for an app that is no longer installed.
  resetExtensionEvents();
  let removed = 0;
  let kept = 0;
  subscribeExtensionEvent("s3disk", "upload.completed", () => {
    removed += 1;
  });
  subscribeExtensionEvent("other", "upload.completed", () => {
    kept += 1;
  });

  removeExtensionEventSubscriptions("s3disk");
  emitExtensionEvent("upload.completed", upload);

  assert.equal(removed, 0);
  assert.equal(kept, 1);
});
