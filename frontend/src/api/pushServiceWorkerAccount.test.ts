import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

type WorkerEvent = {
  data: { json: () => Record<string, unknown> };
  waitUntil: (promise: Promise<unknown>) => void;
};

function workerHarness(response: { status: number; owned?: boolean }) {
  const listeners = new Map<string, (event: WorkerEvent) => void>();
  let displayed = 0;
  let unsubscribed = 0;

  const subscription = {
    endpoint: "https://push.example.com/browser-device",
    unsubscribe: async () => {
      unsubscribed++;
      return true;
    },
  };
  const worker = {
    registration: {
      pushManager: { getSubscription: async () => subscription },
      showNotification: async () => {
        displayed++;
      },
    },
    clients: { claim: async () => {} },
    skipWaiting: () => {},
    addEventListener: (type: string, listener: (event: WorkerEvent) => void) => {
      listeners.set(type, listener);
    },
  };
  const script = readFileSync(new URL("../../public/sw.js", import.meta.url), "utf8");
  vm.runInNewContext(script, {
    self: worker,
    fetch: async () => ({
      status: response.status,
      ok: response.status >= 200 && response.status < 300,
      json: async () => ({ owned: response.owned }),
    }),
    JSON,
    Date,
    URL,
    setTimeout,
    clearTimeout,
  });

  return {
    async receivePush(): Promise<void> {
      let work: Promise<unknown> | undefined;
      listeners.get("push")?.({
        data: { json: () => ({ title: "Private notification" }) },
        waitUntil: (promise) => {
          work = Promise.resolve(promise);
        },
      });
      await work;
    },
    displayed: () => displayed,
    unsubscribed: () => unsubscribed,
  };
}

test("the worker displays a push only when the current account owns its endpoint", async () => {
  const harness = workerHarness({ status: 200, owned: true });

  await harness.receivePush();

  assert.equal(harness.displayed(), 1);
  assert.equal(harness.unsubscribed(), 0);
});

test("the worker drops and invalidates a previous account's endpoint", async () => {
  const harness = workerHarness({ status: 200, owned: false });

  await harness.receivePush();

  assert.equal(harness.displayed(), 0);
  assert.equal(harness.unsubscribed(), 1);
});

test("the worker stays quiet without a session, keeping the device registered", async () => {
  const harness = workerHarness({ status: 401 });

  await harness.receivePush();

  assert.equal(harness.displayed(), 0);
  assert.equal(harness.unsubscribed(), 0);
});

test("the worker fails closed without deleting a subscription on a transient error", async () => {
  const harness = workerHarness({ status: 503 });

  await harness.receivePush();

  assert.equal(harness.displayed(), 0);
  assert.equal(harness.unsubscribed(), 0);
});
