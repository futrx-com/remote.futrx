import assert from "node:assert/strict";
import test from "node:test";

import { pushDeviceOptIn } from "./pushDeviceOptIn.ts";

function useMemoryStorage(): Map<string, string> {
  const entries = new Map<string, string>();
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => entries.get(key) ?? null,
      setItem: (key: string, value: string) => {
        entries.set(key, value);
      },
      removeItem: (key: string) => {
        entries.delete(key);
      },
    },
  });
  return entries;
}

test("an account that turned notifications on is remembered for this browser", async () => {
  useMemoryStorage();

  await pushDeviceOptIn.remember("Person@Example.com");

  assert.equal(await pushDeviceOptIn.has("person@example.com"), true);
});

test("the stored record never contains the account's address", async () => {
  const entries = useMemoryStorage();

  await pushDeviceOptIn.remember("Person@Example.com");

  const stored = [...entries.values()].join(" ").toLowerCase();
  assert.ok(stored.length > 0, "an opt-in was stored");
  assert.ok(!stored.includes("person"), "no local part at rest");
  assert.ok(!stored.includes("example.com"), "no domain at rest");
});

test("one account's opt-in never speaks for another in a shared browser", async () => {
  useMemoryStorage();

  await pushDeviceOptIn.remember("first@example.com");

  assert.equal(await pushDeviceOptIn.has("second@example.com"), false);
});

test("turning notifications off stops the device from being restored", async () => {
  useMemoryStorage();
  await pushDeviceOptIn.remember("first@example.com");
  await pushDeviceOptIn.remember("second@example.com");

  await pushDeviceOptIn.forget("first@example.com");

  assert.equal(await pushDeviceOptIn.has("first@example.com"), false);
  assert.equal(await pushDeviceOptIn.has("second@example.com"), true);
});

test("an unusable store is not an opt-in", async () => {
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    get() {
      throw new Error("blocked");
    },
  });

  await pushDeviceOptIn.remember("first@example.com");

  assert.equal(await pushDeviceOptIn.has("first@example.com"), false);
});
