import assert from "node:assert/strict";
import test from "node:test";

import { formatUpdateRelativeTime } from "./updateTime.ts";

test("formats update age with the existing rounded boundaries", () => {
  const now = 1_000_000;

  assert.equal(formatUpdateRelativeTime(996, now), "just now");
  assert.equal(formatUpdateRelativeTime(995, now), "5 seconds ago");
  assert.equal(formatUpdateRelativeTime(940, now), "1 minute ago");
  assert.equal(formatUpdateRelativeTime(880, now), "2 minutes ago");
  assert.equal(formatUpdateRelativeTime(1_005, now), "just now");
});
