import assert from "node:assert/strict";
import test from "node:test";
import { chatJumpRequestService } from "./chatJumpRequestService.ts";

const hit = { turnId: "t", itemId: "1", startSeq: 5, endSeq: 5, kind: "user", t: 1, text: "x", highlights: [] };

test("a jump request is seen only by its chat and consumed once", () => {
  let notified = 0;
  const unsubscribe = chatJumpRequestService.subscribe(() => { notified++; });
  const jump = { chatId: "abcd", query: "x", hit };
  chatJumpRequestService.request(jump);
  assert.equal(chatJumpRequestService.peek("ffff"), null);
  assert.equal(chatJumpRequestService.peek("abcd"), jump);
  assert.equal(chatJumpRequestService.take(jump), true);
  assert.equal(chatJumpRequestService.take(jump), false);
  assert.equal(chatJumpRequestService.peek("abcd"), null);
  assert.equal(notified, 2);
  unsubscribe();
});

test("a newer request replaces one not yet consumed", () => {
  const first = { chatId: "abcd", query: "x", hit };
  const second = { chatId: "beef", query: "y", hit };
  chatJumpRequestService.request(first);
  chatJumpRequestService.request(second);
  assert.equal(chatJumpRequestService.take(first), false);
  assert.equal(chatJumpRequestService.peek("beef"), second);
  chatJumpRequestService.take(second);
});
