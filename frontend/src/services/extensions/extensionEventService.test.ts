import assert from "node:assert/strict";
import test, { type TestContext } from "node:test";

import { extensionEventService } from "./extensionEventService.ts";
import type {
  ExtensionEventHandler,
  UploadCompletedEvent,
} from "../../models/extension.ts";

const upload: UploadCompletedEvent = {
  chatId: "chat-1",
  projectId: "project-1",
  fileName: "shot-a8ho.png",
  directory: "/workspace/.uploads",
  path: "/workspace/.uploads/shot-a8ho.png",
  size: 233361,
  claim: () => {},
};

/**
 * Subscribes through the app-wide instance and drops the image when the test
 * ends, the way the host does when an image is uninstalled.
 */
function subscribe(
  t: TestContext,
  imageId: string,
  handler: ExtensionEventHandler<"upload.completed">,
): () => void {
  t.after(() => extensionEventService.removeImage(imageId));
  return extensionEventService.on(imageId, "upload.completed", handler);
}

test("a handler receives the events it subscribed to", (t) => {
  const seen: UploadCompletedEvent[] = [];
  subscribe(t, "s3disk", (event) => {
    seen.push(event);
  });

  extensionEventService.emit("upload.completed", upload);

  assert.deepEqual(seen, [upload]);
});

test("unsubscribing stops delivery", (t) => {
  let calls = 0;
  const off = subscribe(t, "s3disk", () => {
    calls += 1;
  });

  extensionEventService.emit("upload.completed", upload);
  off();
  extensionEventService.emit("upload.completed", upload);

  assert.equal(calls, 1);
});

test("a throwing handler does not stop the ones after it", (t) => {
  // The emitter sits inside the upload's onSuccess, so one extension throwing
  // has to cost that extension and nothing else.
  t.mock.method(console, "error", () => {});
  let reached = false;
  subscribe(t, "broken", () => {
    throw new Error("boom");
  });
  subscribe(t, "s3disk", () => {
    reached = true;
  });

  assert.doesNotThrow(() =>
    extensionEventService.emit("upload.completed", upload),
  );
  assert.equal(reached, true);
});

test("a handler may unsubscribe itself mid-emit", (t) => {
  let second = 0;
  const off = subscribe(t, "s3disk", () => {
    off();
  });
  subscribe(t, "other", () => {
    second += 1;
  });

  extensionEventService.emit("upload.completed", upload);

  assert.equal(second, 1);
});

test("removing an image drops its subscriptions and only its own", (t) => {
  // An uninstalled image keeps its entry module in the page, so its handlers
  // would otherwise keep firing for an app that is no longer installed.
  let removed = 0;
  let kept = 0;
  subscribe(t, "s3disk", () => {
    removed += 1;
  });
  subscribe(t, "other", () => {
    kept += 1;
  });

  extensionEventService.removeImage("s3disk");
  extensionEventService.emit("upload.completed", upload);

  assert.equal(removed, 0);
  assert.equal(kept, 1);
});
