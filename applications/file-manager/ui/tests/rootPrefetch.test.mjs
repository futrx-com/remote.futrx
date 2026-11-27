import assert from "node:assert/strict";
import test from "node:test";
import { createRootPrefetch, withPrefetchedRoot } from "../scripts/services/rootPrefetchService.js";

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

test("a failed prefetch is quiet until a drawer consumes it", async () => {
  const prefetch = createRootPrefetch(() => Promise.reject(new Error("down")));
  prefetch.warm("p1");
  await assert.rejects(prefetch.take("p1"), /down/);
});

test("the cached root is consumed once while nested listings and refresh keep their signal", async () => {
  const root = { entries: ["prefetched"] };
  const prefetched = createRootPrefetch(async () => root);
  prefetched.warm("p1");
  const calls = [];
  const signal = new AbortController().signal;
  const files = withPrefetchedRoot({
    list: async (path, givenSignal) => { calls.push([path, givenSignal]); return { path }; },
    search: () => "search",
    downloadUrl: () => "download",
  }, prefetched, "p1");
  assert.deepEqual(await files.list("src", signal), { path: "src" });
  assert.equal(await files.list("", signal), root);
  assert.deepEqual(await files.list("", signal), { path: "" });
  assert.deepEqual(calls, [["src", signal], ["", signal]]);
  assert.equal(files.search(), "search");
  assert.equal(files.downloadUrl(), "download");
});

test("a failed cached root rejects the first open and the next root listing fetches again", async () => {
  const prefetched = createRootPrefetch(async () => { throw new Error("down"); });
  prefetched.warm("p1");
  let calls = 0;
  const files = withPrefetchedRoot({ list: async () => { calls++; return "fresh"; } }, prefetched, "p1");
  await assert.rejects(files.list(""), /down/);
  assert.equal(calls, 0);
  assert.equal(await files.list(""), "fresh");
  assert.equal(calls, 1);
});
