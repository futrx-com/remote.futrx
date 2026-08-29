import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

type ChangeEvent = {
  oldSubscription?: unknown;
  newSubscription?: unknown;
  waitUntil: (promise: Promise<unknown>) => void;
};

interface Call {
  url: string;
  method: string;
  body: unknown;
}

function subscriptionAt(endpoint: string, applicationServerKey?: ArrayBuffer) {
  return {
    endpoint,
    options: applicationServerKey ? { applicationServerKey } : {},
    toJSON: () => ({ endpoint, keys: { p256dh: "p256dh", auth: "auth" } }),
  };
}

const RETIRED = "https://push.example.com/retired";
const REPLACEMENT = "https://push.example.com/replacement";

function workerHarness(options: { owned?: boolean; statusCode?: number } = {}) {
  const listeners = new Map<string, (event: ChangeEvent) => void>();
  const calls: Call[] = [];
  let created = 0;

  const worker = {
    registration: {
      pushManager: {
        getSubscription: async () => null,
        subscribe: async (subscribeOptions: { applicationServerKey: unknown }) => {
          assert.ok(subscribeOptions.applicationServerKey, "subscribe needs a server key");
          created++;
          return subscriptionAt(REPLACEMENT);
        },
      },
      showNotification: async () => {},
    },
    clients: { claim: async () => {} },
    skipWaiting: () => {},
    addEventListener: (type: string, listener: (event: ChangeEvent) => void) => {
      listeners.set(type, listener);
    },
  };

  const script = readFileSync(new URL("../../public/sw.js", import.meta.url), "utf8");
  vm.runInNewContext(script, {
    self: worker,
    fetch: async (url: string, init: { method?: string; body?: string } = {}) => {
      calls.push({
        url,
        method: init.method ?? "GET",
        body: init.body ? JSON.parse(init.body) : undefined,
      });
      const status = options.statusCode ?? 200;
      return {
        status,
        ok: status >= 200 && status < 300,
        json: async () => ({ owned: options.owned ?? true }),
      };
    },
    JSON,
    Date,
    URL,
    setTimeout,
    clearTimeout,
  });

  return {
    async rotate(event: Omit<ChangeEvent, "waitUntil">): Promise<void> {
      let work: Promise<unknown> | undefined;
      listeners.get("pushsubscriptionchange")?.({
        ...event,
        waitUntil: (promise) => {
          work = Promise.resolve(promise);
        },
      });
      await work;
    },
    calls: () => calls,
    created: () => created,
  };
}

test("a retired endpoint is replaced and re-registered without the user acting", async () => {
  const harness = workerHarness();

  await harness.rotate({ oldSubscription: subscriptionAt(RETIRED, new ArrayBuffer(65)) });

  const [ownership, registration, removal] = harness.calls();
  assert.equal(harness.created(), 1);
  assert.equal(ownership.url, "/api/push/subscriptions/status");
  assert.equal(registration.method, "POST");
  assert.equal((registration.body as { endpoint: string }).endpoint, REPLACEMENT);
  // The retired endpoint would otherwise sit in the account's registrations
  // forever, unreachable.
  assert.equal(removal.method, "DELETE");
  assert.deepEqual(removal.body, { endpoint: RETIRED });
});

test("a replacement the browser already made is registered as it is", async () => {
  const harness = workerHarness();

  await harness.rotate({
    oldSubscription: subscriptionAt(RETIRED, new ArrayBuffer(65)),
    newSubscription: subscriptionAt("https://push.example.com/browser-made"),
  });

  assert.equal(harness.created(), 0);
  assert.equal(
    (harness.calls()[1].body as { endpoint: string }).endpoint,
    "https://push.example.com/browser-made"
  );
});

test("an endpoint the signed-in account does not own is not replaced", async () => {
  const harness = workerHarness({ owned: false });

  await harness.rotate({ oldSubscription: subscriptionAt(RETIRED, new ArrayBuffer(65)) });

  assert.equal(harness.created(), 0);
  assert.deepEqual(
    harness.calls().map((call) => call.url),
    ["/api/push/subscriptions/status"]
  );
});

test("a rotation while signed out is left for the app to restore", async () => {
  const harness = workerHarness({ statusCode: 401 });

  await harness.rotate({ oldSubscription: subscriptionAt(RETIRED, new ArrayBuffer(65)) });

  assert.equal(harness.created(), 0);
  assert.deepEqual(
    harness.calls().map((call) => call.url),
    ["/api/push/subscriptions/status"]
  );
});

test("a rotation the browser reports no old endpoint for changes nothing", async () => {
  const harness = workerHarness();

  await harness.rotate({});

  assert.equal(harness.created(), 0);
  assert.deepEqual(harness.calls(), []);
});
