import assert from "node:assert/strict";
import test from "node:test";

import { settleClaims } from "./attachmentClaimState.ts";

const MOVED = "/workspace/s3/uploads/shot-a8ho.png";

test("no claims means nothing to wait for and nothing to move", async () => {
  assert.equal(await settleClaims([]), null);
});

test("a claim resolving with a path re-points the prompt", async () => {
  assert.equal(await settleClaims([Promise.resolve(MOVED)]), MOVED);
});

test("a claim that resolves with nothing leaves the attachment alone", async () => {
  // The extension looked at the upload but did not move it, so the path the
  // composer already has is still the right one.
  assert.equal(await settleClaims([Promise.resolve()]), null);
});

test("a rejected claim leaves the attachment where it is", async () => {
  // The move failed, so .uploads still holds the file — re-pointing the
  // prompt here would hand the agent a path to nothing.
  assert.equal(await settleClaims([Promise.reject(new Error("bucket down"))]), null);
});

test("one failing claim does not discard another's move", async () => {
  const relocated = await settleClaims([
    Promise.reject(new Error("bucket down")),
    Promise.resolve(MOVED),
  ]);
  assert.equal(relocated, MOVED);
});

test("the last claim to move the file wins", async () => {
  const second = "/workspace/s3/archive/shot-a8ho.png";
  assert.equal(await settleClaims([Promise.resolve(MOVED), Promise.resolve(second)]), second);
});

test("all claims are waited for, not just the first to settle", async () => {
  let finished = false;
  const slow = new Promise<string>((resolve) => {
    setTimeout(() => {
      finished = true;
      resolve(MOVED);
    }, 20);
  });

  const relocated = await settleClaims([Promise.resolve(), slow]);

  assert.equal(finished, true);
  assert.equal(relocated, MOVED);
});

test("a claim that never settles is abandoned rather than wedging the composer", async () => {
  const never = new Promise<string>(() => {});
  assert.equal(await settleClaims([never], 10), null);
});
