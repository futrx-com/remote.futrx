import assert from "node:assert/strict";
import test from "node:test";
import {
  CHAT_INITIAL_TRANSCRIPT_TURN_LIMIT,
  CHAT_TRANSCRIPT_TURN_PAGE_LIMIT,
} from "./api.ts";

test("chat transcript paging starts small and loads larger older pages", () => {
  assert.equal(CHAT_INITIAL_TRANSCRIPT_TURN_LIMIT, 10);
  assert.equal(CHAT_TRANSCRIPT_TURN_PAGE_LIMIT, 20);
  assert.ok(CHAT_INITIAL_TRANSCRIPT_TURN_LIMIT < CHAT_TRANSCRIPT_TURN_PAGE_LIMIT);
});
