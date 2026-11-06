import assert from "node:assert/strict";
import test from "node:test";
import { createRootPrefetch } from "../scripts/api/rootPrefetch.js";

test("a showing chat fetches its root once per window and one drawer uses it", async () => {
  let clock = 0;
  const loads = [];
  const prefetch = createRootPrefetch((projectId) => {
    loads.push(projectId);
    return Promise.resolve({ path: "", entries: [projectId] });
  }, { maxAgeMs: 1000, now: () => clock });

  prefetch.warm("p1");
  prefetch.warm("p1");
  prefetch.warm("p2");
  assert.deepEqual(loads, ["p1", "p2"]);
  assert.deepEqual(await prefetch.take("p1"), { path: "", entries: ["p1"] });
  assert.equal(prefetch.take("p1"), null, "a listing is used once");
  prefetch.warm("p1");
  assert.deepEqual(loads, ["p1", "p2"], "a used listing still throttles refetching");

  clock = 1000;
  assert.equal(prefetch.take("p2"), null, "a stale listing is not shown");
  prefetch.warm("p1");
  assert.deepEqual(loads, ["p1", "p2", "p1"]);
});

test("a failed prefetch is not reported and lets the drawer fetch for itself", async () => {
  const prefetch = createRootPrefetch(() => Promise.reject(new Error("down")));
  prefetch.warm("p1");
  await assert.rejects(prefetch.take("p1"), /down/);
});
