import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

type Shown = { tag: string; title: string; closed: boolean };

// A tray that ignores tags when showing, like iOS: only closing an entry
// removes it.
function workerHarness() {
  const listeners = new Map<string, (event: any) => void>();
  const tray: Shown[] = [];
  const worker = {
    registration: {
      pushManager: { getSubscription: async () => ({ endpoint: "https://push.example.com/d" }) },
      getNotifications: async ({ tag }: { tag: string }) =>
        tray
          .filter((entry) => !entry.closed && entry.tag === tag)
          .map((entry) => ({ close: () => { entry.closed = true; } })),
      showNotification: async (title: string, options: { tag: string }) => {
        tray.push({ title, tag: options.tag, closed: false });
      },
    },
    clients: { claim: async () => {}, matchAll: async () => [] },
    skipWaiting: () => {},
    addEventListener: (type: string, listener: (event: any) => void) => {
      listeners.set(type, listener);
    },
  };
  const script = readFileSync(new URL("../../public/sw.js", import.meta.url), "utf8");
  vm.runInNewContext(script, {
    self: worker,
    fetch: async () => ({ ok: true, json: async () => ({ owned: true }) }),
    JSON, Date, URL, setTimeout, clearTimeout,
  });

  return {
    async receive(payload: Record<string, unknown>): Promise<void> {
      let work: Promise<unknown> | undefined;
      listeners.get("push")?.({
        data: { json: () => payload },
        waitUntil: (promise: Promise<unknown>) => { work = promise; },
      });
      await work;
    },
    open: () => tray.filter((entry) => !entry.closed).map((entry) => entry.title),
  };
}

test("a new notification closes the ones already shown for its chat", async () => {
  const worker = workerHarness();

  await worker.receive({ title: "turn 1", tag: "chat:a", chatId: "a" });
  await worker.receive({ title: "other chat", tag: "chat:b", chatId: "b" });
  await worker.receive({ title: "turn 2", tag: "chat:a", chatId: "a" });

  assert.deepEqual(worker.open(), ["other chat", "turn 2"]);
});
