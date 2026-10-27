import assert from "node:assert/strict";
import test from "node:test";

import { normalizeProjectContainerInfo } from "./projectContainerInfo.ts";

const baseInfo = {
  name: "web",
  state: "RUNNING" as const,
  bootAutostart: true,
  claude: {
    installed: true,
    claudeMdInstalled: true,
    claudeMdInSync: true,
  },
  codex: { installed: true },
};

test("container info normalizes nullable auth bundle collections", () => {
  const info = normalizeProjectContainerInfo({
    ...baseInfo,
    authBundles: [
      { name: "claude", files: [] },
      { name: "kimi", files: null },
    ],
  });

  assert.deepEqual(info.authBundles, [
    { name: "claude", files: [] },
    { name: "kimi", files: [] },
  ]);
});

test("container info normalizes a nullable auth bundle list", () => {
  const info = normalizeProjectContainerInfo({
    ...baseInfo,
    authBundles: null,
  });

  assert.deepEqual(info.authBundles, []);
});
