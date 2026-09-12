import assert from "node:assert/strict";
import test from "node:test";
import { chatDeepLinkState } from "./chatDeepLink.ts";

test("reads a well-formed chat id from the query string", () => {
  assert.equal(chatDeepLinkState.parse("?chat=abc123"), "abc123");
  assert.equal(chatDeepLinkState.parse("?foo=1&chat=abc123&bar=2"), "abc123");
  assert.equal(chatDeepLinkState.parse("?chat=ABC123"), "abc123");
  assert.equal(chatDeepLinkState.parse("?chat=%20abc123%20"), "abc123");
});

test("ignores anything that is not a chat id", () => {
  assert.equal(chatDeepLinkState.parse(""), null);
  assert.equal(chatDeepLinkState.parse("?other=abc123"), null);
  assert.equal(chatDeepLinkState.parse("?chat="), null);
  assert.equal(chatDeepLinkState.parse("?chat=abc"), null, "too short");
  assert.equal(chatDeepLinkState.parse(`?chat=${"a".repeat(33)}`), null, "too long");
  assert.equal(chatDeepLinkState.parse("?chat=../../etc/passwd"), null);
  assert.equal(chatDeepLinkState.parse("?chat=abc-123"), null, "non-hex characters");
  assert.equal(chatDeepLinkState.parse("?chat=https://evil.example.com"), null);
});

test("strips only the chat parameter when the link has been consumed", () => {
  assert.equal(chatDeepLinkState.withoutChatParam("/", "?chat=abc123", ""), "/");
  assert.equal(
    chatDeepLinkState.withoutChatParam("/", "?chat=abc123&keep=1", ""),
    "/?keep=1"
  );
  assert.equal(chatDeepLinkState.withoutChatParam("/", "", "#top"), "/#top");
  assert.equal(
    chatDeepLinkState.withoutChatParam("/", "?chat=abc123", "#top"),
    "/#top"
  );
});
