import assert from "node:assert/strict";
import test, { type TestContext } from "node:test";

import { announceUpload } from "./attachmentClaimPolicy.ts";
import type {
  CompletedUpload,
  UploadCompletedEvent,
} from "../../../models/extension.ts";
import { extensionEventService } from "../../../services/extensions/extensionEventService.ts";

const UPLOAD: CompletedUpload = {
  chatId: "chat-1",
  projectId: "project-1",
  fileName: "shot-a8ho.png",
  directory: "/workspace/.uploads",
  path: "/workspace/.uploads/shot-a8ho.png",
  size: 233361,
};
const MOVED = "/workspace/s3/uploads/shot-a8ho.png";

/** Installs an extension for one test; the service is the app-wide instance. */
function extension(
  t: TestContext,
  imageId: string,
  handler: (event: UploadCompletedEvent) => void,
): void {
  extensionEventService.on(imageId, "upload.completed", handler);
  t.after(() => extensionEventService.removeImage(imageId));
}

/** Installs one that takes the attachment over with `work`. */
function claimant(
  t: TestContext,
  imageId: string,
  work: Promise<string | void>,
): void {
  extension(t, imageId, (event) => {
    event.claim(work);
  });
}

test("an upload nothing claimed is finished the moment it is announced", () => {
  assert.equal(announceUpload(UPLOAD), null);
});

test("an extension that only watches is handed the upload, not waited for", (t) => {
  const seen: UploadCompletedEvent[] = [];
  extension(t, "s3disk", (event) => {
    seen.push(event);
  });

  const claimed = announceUpload(UPLOAD);

  assert.equal(claimed, null);
  assert.deepEqual(seen, [{ ...UPLOAD, claim: seen[0]?.claim }]);
});

test("a claim resolving with a path re-points the prompt", async (t) => {
  claimant(t, "s3disk", Promise.resolve(MOVED));

  assert.equal(await announceUpload(UPLOAD), MOVED);
});

test("a claim that resolves with nothing leaves the attachment alone", async (t) => {
  // The extension looked at the upload but did not move it, so the path the
  // composer already has is still the right one.
  claimant(t, "s3disk", Promise.resolve());

  assert.equal(await announceUpload(UPLOAD), null);
});

test("a rejected claim leaves the attachment where it is", async (t) => {
  // The move failed, so .uploads still holds the file — re-pointing the
  // prompt here would hand the agent a path to nothing.
  claimant(t, "s3disk", Promise.reject(new Error("bucket down")));

  assert.equal(await announceUpload(UPLOAD), null);
});

test("one failing claim does not discard another's move", async (t) => {
  claimant(t, "broken", Promise.reject(new Error("bucket down")));
  claimant(t, "s3disk", Promise.resolve(MOVED));

  assert.equal(await announceUpload(UPLOAD), MOVED);
});

test("the last claim to move the file wins", async (t) => {
  const archived = "/workspace/s3/archive/shot-a8ho.png";
  claimant(t, "s3disk", Promise.resolve(MOVED));
  claimant(t, "archiver", Promise.resolve(archived));

  assert.equal(await announceUpload(UPLOAD), archived);
});

test("every claim is waited for, not just the first to settle", async (t) => {
  let finished = false;
  const slow = new Promise<string>((resolve) => {
    setTimeout(() => {
      finished = true;
      resolve(MOVED);
    }, 20);
  });
  claimant(t, "watcher", Promise.resolve());
  claimant(t, "s3disk", slow);

  const relocated = await announceUpload(UPLOAD);

  assert.equal(finished, true);
  assert.equal(relocated, MOVED);
});

test("a claim that never settles is abandoned rather than wedging the composer", async (t) => {
  claimant(t, "s3disk", new Promise<string>(() => {}));

  assert.equal(await announceUpload(UPLOAD, 10), null);
});
