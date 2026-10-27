import assert from "node:assert/strict";
import test from "node:test";
import { PROJECT_RESERVED_PREVIEW_PORTS } from "../../config/project.ts";
import type { ContainerApp, ProjectShare } from "../../models/project.ts";
import { projectShareService } from "./projectShareService.ts";

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

test("rejects ports outside the preview range and reserved platform ports", () => {
  const apps: ContainerApp[] = [
    { port: 3000 },
    { port: 1024 },
    { port: 65535 },
    { port: 1023 },
    { port: 65536 },
    { port: PROJECT_RESERVED_PREVIEW_PORTS.agentBrowser },
    { port: 3000.5 },
  ];

  assert.deepEqual(projectShareService.portRows(apps, []), [
    { port: 1024, process: undefined, shareCount: 0 },
    { port: 3000, process: undefined, shareCount: 0 },
    { port: 65535, process: undefined, shareCount: 0 },
  ]);
});

test("lists shareable ports with their link counts", () => {
  const apps: ContainerApp[] = [
    { port: 5173, process: "vite" },
    { port: PROJECT_RESERVED_PREVIEW_PORTS.agentBrowser, process: "novnc" },
    { port: 3000, process: "node" },
    { port: 3000, process: "node" },
  ];
  const shares = [
    share({ id: "a", port: 3000 }),
    share({ id: "b", port: 3000 }),
    share({ id: "c", port: 8080 }),
  ];

  assert.deepEqual(projectShareService.portRows(apps, shares), [
    { port: 3000, process: "node", shareCount: 2 },
    { port: 5173, process: "vite", shareCount: 0 },
    { port: 8080, shareCount: 1 },
  ]);
});

test("keeps expired links out of the live list and orders the rest newest first", () => {
  const stale = share({ id: "stale", expiresAt: now - hour });
  const older = share({ id: "older", createdAt: now - 2 * hour });
  const newer = share({ id: "newer", createdAt: now - hour });

  assert.deepEqual(
    projectShareService.live([older, stale, newer], now).map((entry) => entry.id),
    ["newer", "older"],
  );
});

test("adds and removes links without duplicating an id", () => {
  const existing = [share({ id: "a" }), share({ id: "b" })];
  const replaced = projectShareService.add(existing, share({ id: "a", port: 5173 }));

  assert.deepEqual(replaced.map((entry) => entry.id), ["a", "b"]);
  assert.equal(replaced[0].port, 5173);
  assert.deepEqual(projectShareService.remove(replaced, "a").map((entry) => entry.id), ["b"]);
});

test("describes remaining lifetime in the largest sensible unit", () => {
  assert.equal(projectShareService.formatExpiry(now - 1, now), "expired");
  assert.equal(projectShareService.formatExpiry(now + 30 * 1000, now), "expires in 1m");
  assert.equal(projectShareService.formatExpiry(now + 45 * 60 * 1000, now), "expires in 45m");
  assert.equal(projectShareService.formatExpiry(now + 5 * hour, now), "expires in 5h");
  assert.equal(projectShareService.formatExpiry(now + 168 * hour, now), "expires in 7d");
});

test("summarizes the active link count", () => {
  assert.equal(projectShareService.describeCount(0), "No active public links");
  assert.equal(projectShareService.describeCount(1), "1 active public link");
  assert.equal(projectShareService.describeCount(3), "3 active public links");
});
