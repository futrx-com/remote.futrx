import assert from "node:assert/strict";
import test from "node:test";
import { CHAT_TITLE_MAX_LENGTH } from "../../../config/chat.ts";
import { chatTitleState } from "./chatTitleState.ts";

test("rename trims and collapses whitespace", () => {
  assert.equal(chatTitleState.rename("  Fix   the\nlogin  bug ", "Old"), "Fix the login bug");
});

test("rename refuses an empty or whitespace-only title", () => {
  assert.equal(chatTitleState.rename("", "Old"), null);
  assert.equal(chatTitleState.rename("   \n\t ", "Old"), null);
});

test("rename returns null when nothing changed", () => {
  assert.equal(chatTitleState.rename("Old", "Old"), null);
  assert.equal(chatTitleState.rename("  Old  ", "Old"), null);
});

test("rename accepts a title for a chat that has none yet", () => {
  assert.equal(chatTitleState.rename("First title", undefined), "First title");
  assert.equal(chatTitleState.rename("First title", ""), "First title");
});

test("rename caps the length without leaving a trailing space", () => {
  const long = "a".repeat(CHAT_TITLE_MAX_LENGTH - 1) + " bcd";
  const result = chatTitleState.rename(long, "Old");
  assert.equal(result, "a".repeat(CHAT_TITLE_MAX_LENGTH - 1));
  assert.ok((result ?? "").length <= CHAT_TITLE_MAX_LENGTH);
});
