import assert from "node:assert/strict";
import test from "node:test";
import type { ContainerApp, ProjectShare } from "../../models/project.ts";
import {
  AGENT_BROWSER_PORT,
  addShare,
  describeShareCount,
  formatShareExpiry,
  isShareExpired,
  isShareablePort,
  liveShares,
  removeShare,
  shareablePortRows,
} from "./projectShareState.ts";

const now = Date.UTC(2026, 2, 1, 12, 0, 0);
const hour = 60 * 60 * 1000;

function share(overrides: Partial<ProjectShare> = {}): ProjectShare {
  return {
    id: "share-1",
    port: 3000,
    createdAt: now,
    expiresAt: now + 24 * hour,
    ...overrides,
  };
}

test("rejects ports outside the preview range and the agent browser port", () => {
  assert.equal(isShareablePort(3000), true);
  assert.equal(isShareablePort(1024), true);
  assert.equal(isShareablePort(65535), true);
  assert.equal(isShareablePort(1023), false);
  assert.equal(isShareablePort(65536), false);
  assert.equal(isShareablePort(AGENT_BROWSER_PORT), false);
  assert.equal(isShareablePort(3000.5), false);
});

test("lists shareable ports with their live link counts", () => {
  const apps: ContainerApp[] = [
    { port: 5173, process: "vite" },
    { port: AGENT_BROWSER_PORT, process: "novnc" },
    { port: 3000, process: "node" },
    { port: 3000, process: "node" },
  ];
  const shares = [
    share({ id: "a", port: 3000 }),
    share({ id: "b", port: 3000 }),
    share({ id: "c", port: 8080 }),
  ];

  assert.deepEqual(shareablePortRows(apps, shares), [
    { port: 3000, process: "node", shareCount: 2 },
    { port: 5173, process: "vite", shareCount: 0 },
    { port: 8080, shareCount: 1 },
  ]);
});

test("keeps expired links out of the live list and orders the rest newest first", () => {
  const stale = share({ id: "stale", expiresAt: now - hour });
  const older = share({ id: "older", createdAt: now - 2 * hour });
  const newer = share({ id: "newer", createdAt: now - hour });

  assert.equal(isShareExpired(stale, now), true);
  assert.deepEqual(
    liveShares([older, stale, newer], now).map((entry) => entry.id),
    ["newer", "older"],
  );
});

test("adds and removes links without duplicating an id", () => {
  const existing = [share({ id: "a" }), share({ id: "b" })];
  const replaced = addShare(existing, share({ id: "a", port: 5173 }));

  assert.deepEqual(replaced.map((entry) => entry.id), ["a", "b"]);
  assert.equal(replaced[0].port, 5173);
  assert.deepEqual(removeShare(replaced, "a").map((entry) => entry.id), ["b"]);
});

test("describes remaining lifetime in the largest sensible unit", () => {
  assert.equal(formatShareExpiry(now - 1, now), "expired");
  assert.equal(formatShareExpiry(now + 30 * 1000, now), "expires in 1m");
  assert.equal(formatShareExpiry(now + 45 * 60 * 1000, now), "expires in 45m");
  assert.equal(formatShareExpiry(now + 5 * hour, now), "expires in 5h");
  assert.equal(formatShareExpiry(now + 168 * hour, now), "expires in 7d");
});

test("summarizes the active link count", () => {
  assert.equal(describeShareCount(0), "No active public links");
  assert.equal(describeShareCount(1), "1 active public link");
  assert.equal(describeShareCount(3), "3 active public links");
});
