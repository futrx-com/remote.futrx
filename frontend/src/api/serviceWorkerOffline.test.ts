import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

type WorkerListener = (event: any) => void;

const CURRENT_CACHE = "remote-futrx-offline-v1";

function workerHarness(initialCacheKeys: string[] = []) {
  const listeners = new Map<string, WorkerListener>();
  const cacheKeys = new Set(initialCacheKeys);
  const deletedCaches: string[] = [];
  const addedRequests: Array<{ url: string; cache?: string }> = [];
  const offlineResponse = { source: "offline cache" };
  const networkResponse = { source: "network" };
  let claims = 0;
  let skipWaitingCalls = 0;
  let networkFails = false;

  class Request {
    url: string;
    cache?: string;

    constructor(url: string, options?: { cache?: string }) {
      this.url = url;
      this.cache = options?.cache;
    }
  }

  const caches = {
    keys: async () => [...cacheKeys],
    delete: async (key: string) => {
      deletedCaches.push(key);
      return cacheKeys.delete(key);
    },
    open: async (key: string) => {
      cacheKeys.add(key);
      return {
        add: async (request: Request) => {
          addedRequests.push({ url: request.url, cache: request.cache });
        },
        match: async (url: string) =>
          key === CURRENT_CACHE && url === "/offline.html" ? offlineResponse : undefined,
      };
    },
  };

  const worker = {
    registration: {
      pushManager: { getSubscription: async () => null },
      showNotification: async () => {},
    },
    clients: {
      claim: async () => {
        claims++;
      },
    },
    skipWaiting: () => {
      skipWaitingCalls++;
    },
    addEventListener: (type: string, listener: WorkerListener) => {
      listeners.set(type, listener);
    },
  };

  const script = readFileSync(new URL("../../public/sw.js", import.meta.url), "utf8");
  vm.runInNewContext(script, {
    self: worker,
    caches,
    fetch: async () => {
      if (networkFails) throw new Error("offline");
      return networkResponse;
    },
    Request,
    Response: { error: () => ({ source: "error" }) },
    MessageChannel,
    JSON,
    Date,
    URL,
    setTimeout,
    clearTimeout,
  });

  async function dispatchExtendableEvent(type: "install" | "activate") {
    let work: Promise<unknown> | undefined;
    listeners.get(type)?.({
      waitUntil: (promise: Promise<unknown>) => {
        work = Promise.resolve(promise);
      },
    });
    await work;
  }

  return {
    install: () => dispatchExtendableEvent("install"),
    activate: () => dispatchExtendableEvent("activate"),
    async fetch(mode: string) {
      let response: Promise<unknown> | undefined;
      listeners.get("fetch")?.({
        request: { mode },
        respondWith: (promise: Promise<unknown>) => {
          response = Promise.resolve(promise);
        },
      });
      return response;
    },
    failNetwork: () => {
      networkFails = true;
    },
    cacheKeys: () => [...cacheKeys],
    deletedCaches: () => deletedCaches,
    addedRequests: () => addedRequests,
    claims: () => claims,
    skipWaitingCalls: () => skipWaitingCalls,
    offlineResponse,
    networkResponse,
  };
}

test("install stores a fresh offline page in the Remote-owned cache", async () => {
  const harness = workerHarness();

  await harness.install();

  assert.equal(harness.skipWaitingCalls(), 1);
  assert.deepEqual(harness.cacheKeys(), [CURRENT_CACHE]);
  assert.deepEqual(harness.addedRequests(), [{ url: "/offline.html", cache: "reload" }]);
});

test("activate deletes only stale Remote-owned offline caches", async () => {
  const harness = workerHarness([
    "offline-v1",
    "remote-futrx-offline-v0",
    CURRENT_CACHE,
    "offline-v2",
    "image-thumbnails-v3",
  ]);

  await harness.activate();

  assert.deepEqual(harness.deletedCaches().sort(), ["offline-v1", "remote-futrx-offline-v0"]);
  assert.deepEqual(
    harness.cacheKeys().sort(),
    [CURRENT_CACHE, "image-thumbnails-v3", "offline-v2"].sort()
  );
  assert.equal(harness.claims(), 1);
});

test("fetch uses the network for navigations and the current offline cache on failure", async () => {
  const harness = workerHarness([CURRENT_CACHE]);

  assert.equal(await harness.fetch("navigate"), harness.networkResponse);

  harness.failNetwork();
  assert.equal(await harness.fetch("navigate"), harness.offlineResponse);
  assert.equal(await harness.fetch("cors"), undefined);
});
