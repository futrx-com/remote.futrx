import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

function workerHarness(windowPaths: string[]) {
  const listeners = new Map<string, (event: any) => void>();
  const opened: string[] = [];
  const navigated: string[] = [];
  const focused: string[] = [];
  const clients = windowPaths.map((path) => ({
    url: `https://remote.example${path}`,
    focus: async () => { focused.push(path); },
    navigate: async (target: string) => {
      navigated.push(target);
      return { focus: async () => { focused.push(target); } };
    },
  }));
  const self = {
    location: { origin: "https://remote.example" },
    clients: {
      matchAll: async () => clients,
      openWindow: async (path: string) => { opened.push(path); },
    },
    addEventListener: (type: string, callback: (event: any) => void) => { listeners.set(type, callback); },
  };
  const script = readFileSync(new URL("../../public/sw.js", import.meta.url), "utf8");
  vm.runInNewContext(script, { self, URL, JSON, Date, setTimeout, clearTimeout });
  return {
    async click(chatId: string) {
      let work: Promise<unknown> | undefined;
      listeners.get("notificationclick")?.({
        notification: { close: () => {}, data: { chatId } },
        waitUntil: (promise: Promise<unknown>) => { work = promise; },
      });
      await work;
    },
    opened, navigated, focused,
  };
}

test("a notification opens its exact chat on cold start", async () => {
  const worker = workerHarness([]);
  await worker.click("abcdef12");
  assert.deepEqual(worker.opened, ["/chats/abcdef12"]);
});

test("a notification reuses a window already showing that chat", async () => {
  const worker = workerHarness(["/settings", "/chats/abcdef12"]);
  await worker.click("abcdef12");
  assert.deepEqual(worker.focused, ["/chats/abcdef12"]);
  assert.deepEqual(worker.navigated, []);
});

test("a notification navigates an existing window to its chat", async () => {
  const worker = workerHarness(["/settings"]);
  await worker.click("abcdef12");
  assert.deepEqual(worker.navigated, ["/chats/abcdef12"]);
  assert.deepEqual(worker.focused, ["/chats/abcdef12"]);
});
