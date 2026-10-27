import assert from "node:assert/strict";
import test, { type TestContext } from "node:test";
import { pushApi } from "../../../api/pushApi.ts";
import { PUSH_PRESENCE_HEARTBEAT_MS } from "../../../config/push.ts";
import type { PushPresencePayload } from "../../../models/push.ts";

type PresenceStoreModule = typeof import("./pushPresenceStore.ts");
type Listener = () => void;

interface PresenceCall {
  payload: PushPresencePayload;
  keepalive: boolean | undefined;
}

interface RecordedInterval {
  id: number;
  callback: Listener;
  delay: number | undefined;
  cleared: boolean;
}

/** Loads a fresh singleton so module-level timers and listeners cannot leak between cases. */
async function loadPresenceStore(caseName: string): Promise<PresenceStoreModule> {
  const url = new URL("./pushPresenceStore.ts", import.meta.url);
  url.searchParams.set("case", caseName);
  return (await import(url.href)) as PresenceStoreModule;
}

function replaceGlobal(t: TestContext, key: "window" | "document" | "clearInterval", value: unknown) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, key);
  Object.defineProperty(globalThis, key, {
    configurable: true,
    writable: true,
    value,
  });
  t.after(() => {
    if (previous) Object.defineProperty(globalThis, key, previous);
    else Reflect.deleteProperty(globalThis, key);
  });
}

function removeGlobal(t: TestContext, key: "window" | "document") {
  const previous = Object.getOwnPropertyDescriptor(globalThis, key);
  Reflect.deleteProperty(globalThis, key);
  t.after(() => {
    if (previous) Object.defineProperty(globalThis, key, previous);
    else Reflect.deleteProperty(globalThis, key);
  });
}

function installBrowser(t: TestContext) {
  let focused = true;
  let visibility: DocumentVisibilityState = "visible";
  let nextIntervalId = 1;
  const documentListeners = new Map<string, Listener[]>();
  const windowListeners = new Map<string, Listener[]>();
  const intervals: RecordedInterval[] = [];

  const addListener = (
    listeners: Map<string, Listener[]>,
    type: string,
    listener: EventListenerOrEventListenerObject,
  ) => {
    const invoke = typeof listener === "function"
      ? () => listener(new Event(type))
      : () => listener.handleEvent(new Event(type));
    listeners.set(type, [...(listeners.get(type) ?? []), invoke]);
  };

  const clearRecordedInterval = (id: number | undefined) => {
    const interval = intervals.find((candidate) => candidate.id === id);
    if (interval) interval.cleared = true;
  };

  const fakeDocument = {
    get visibilityState() {
      return visibility;
    },
    hasFocus: () => focused,
    addEventListener: (type: string, listener: EventListenerOrEventListenerObject) => {
      addListener(documentListeners, type, listener);
    },
  } as unknown as Document;

  const fakeWindow = {
    addEventListener: (type: string, listener: EventListenerOrEventListenerObject) => {
      addListener(windowListeners, type, listener);
    },
    setInterval: (callback: Listener, delay?: number) => {
      const interval = { id: nextIntervalId++, callback, delay, cleared: false };
      intervals.push(interval);
      return interval.id;
    },
    clearInterval: clearRecordedInterval,
  } as unknown as Window & typeof globalThis;

  replaceGlobal(t, "document", fakeDocument);
  replaceGlobal(t, "window", fakeWindow);
  // The production store intentionally uses the browser-global spelling.
  replaceGlobal(t, "clearInterval", clearRecordedInterval);

  return {
    intervals,
    activeIntervals: () => intervals.filter((interval) => !interval.cleared),
    listenerCount: (target: "document" | "window", type: string) =>
      (target === "document" ? documentListeners : windowListeners).get(type)?.length ?? 0,
    setFocused: (value: boolean) => {
      focused = value;
    },
    setVisibility: (value: DocumentVisibilityState) => {
      visibility = value;
    },
    dispatchDocument: (type: string) => {
      for (const listener of documentListeners.get(type) ?? []) listener();
    },
    dispatchWindow: (type: string) => {
      for (const listener of windowListeners.get(type) ?? []) listener();
    },
  };
}

function recordPresence(t: TestContext, fail = false): PresenceCall[] {
  const original = pushApi.presence;
  const calls: PresenceCall[] = [];
  pushApi.presence = async (payload, keepalive) => {
    calls.push({ payload: { ...payload }, keepalive });
    if (fail) throw new Error("offline");
  };
  t.after(() => {
    pushApi.presence = original;
  });
  return calls;
}

test("claims a focused visible chat and starts one heartbeat", async (t) => {
  const browser = installBrowser(t);
  const calls = recordPresence(t);
  const { pushPresenceStore } = await loadPresenceStore("initial-claim");

  pushPresenceStore.getState().setWatchedChat("chat-1");

  assert.deepEqual(
    {
      onScreenChatId: pushPresenceStore.getState().onScreenChatId,
      claimedChatId: pushPresenceStore.getState().claimedChatId,
      revision: pushPresenceStore.getState().revision,
    },
    { onScreenChatId: "chat-1", claimedChatId: "chat-1", revision: 1 },
  );
  assert.equal(calls.length, 1);
  assert.equal(calls[0]?.payload.chatId, "chat-1");
  assert.equal(calls[0]?.payload.revision, 1);
  assert.ok(calls[0]?.payload.clientId);
  assert.equal(calls[0]?.keepalive, false);
  assert.equal(browser.activeIntervals().length, 1);
  assert.equal(browser.activeIntervals()[0]?.delay, PUSH_PRESENCE_HEARTBEAT_MS);

  assert.equal(browser.listenerCount("document", "visibilitychange"), 1);
  for (const event of ["focus", "blur", "pagehide"]) {
    assert.equal(browser.listenerCount("window", event), 1);
  }

  browser.dispatchWindow("pagehide");
});

test("repeating the current chat is a network and heartbeat no-op", async (t) => {
  const browser = installBrowser(t);
  const calls = recordPresence(t);
  const { pushPresenceStore } = await loadPresenceStore("repeat-no-op");

  pushPresenceStore.getState().setWatchedChat("chat-1");
  const heartbeat = browser.activeIntervals()[0];
  pushPresenceStore.getState().setWatchedChat("chat-1");

  assert.equal(calls.length, 1);
  assert.equal(pushPresenceStore.getState().revision, 1);
  assert.equal(browser.intervals.length, 1);
  assert.equal(browser.activeIntervals()[0], heartbeat);
  assert.equal(browser.listenerCount("window", "focus"), 1);

  browser.dispatchWindow("pagehide");
});

test("clearing the watched chat withdraws the claim and stops its heartbeat", async (t) => {
  const browser = installBrowser(t);
  const calls = recordPresence(t);
  const { pushPresenceStore } = await loadPresenceStore("clear-chat");

  pushPresenceStore.getState().setWatchedChat("chat-1");
  const heartbeat = browser.activeIntervals()[0];
  pushPresenceStore.getState().setWatchedChat(null);

  assert.equal(pushPresenceStore.getState().onScreenChatId, null);
  assert.equal(pushPresenceStore.getState().claimedChatId, null);
  assert.equal(pushPresenceStore.getState().revision, 2);
  assert.equal(heartbeat?.cleared, true);
  assert.equal(browser.activeIntervals().length, 0);
  assert.deepEqual(
    calls.map(({ payload, keepalive }) => ({ chatId: payload.chatId, keepalive })),
    [
      { chatId: "chat-1", keepalive: false },
      { chatId: "", keepalive: true },
    ],
  );

  // Clearing an already-empty selection cannot send a second withdrawal.
  pushPresenceStore.getState().setWatchedChat(null);
  assert.equal(calls.length, 2);
});

test("blur withdraws with keepalive and focus restores the on-screen claim", async (t) => {
  const browser = installBrowser(t);
  const calls = recordPresence(t);
  const { pushPresenceStore } = await loadPresenceStore("focus-cycle");

  pushPresenceStore.getState().setWatchedChat("chat-1");
  const firstHeartbeat = browser.activeIntervals()[0];

  browser.setFocused(false);
  browser.dispatchWindow("blur");

  assert.equal(pushPresenceStore.getState().onScreenChatId, "chat-1");
  assert.equal(pushPresenceStore.getState().claimedChatId, null);
  assert.equal(pushPresenceStore.getState().revision, 2);
  assert.equal(firstHeartbeat?.cleared, true);
  assert.equal(browser.activeIntervals().length, 0);
  assert.deepEqual(
    { chatId: calls[1]?.payload.chatId, keepalive: calls[1]?.keepalive },
    { chatId: "", keepalive: true },
  );

  // A repeated blur cannot issue a second withdrawal.
  browser.dispatchWindow("blur");
  assert.equal(calls.length, 2);

  browser.setFocused(true);
  browser.dispatchWindow("focus");

  assert.equal(pushPresenceStore.getState().claimedChatId, "chat-1");
  assert.equal(pushPresenceStore.getState().revision, 3);
  assert.equal(calls[2]?.payload.chatId, "chat-1");
  assert.equal(calls[2]?.keepalive, false);
  assert.equal(calls[2]?.payload.clientId, calls[0]?.payload.clientId);
  assert.equal(browser.activeIntervals().length, 1);

  browser.dispatchWindow("pagehide");
});

test("visibility changes withdraw and reclaim the current on-screen chat", async (t) => {
  const browser = installBrowser(t);
  const calls = recordPresence(t);
  const { pushPresenceStore } = await loadPresenceStore("visibility-cycle");

  pushPresenceStore.getState().setWatchedChat("chat-1");
  browser.setVisibility("hidden");
  browser.dispatchDocument("visibilitychange");
  browser.setVisibility("visible");
  browser.dispatchDocument("visibilitychange");

  assert.deepEqual(
    calls.map(({ payload, keepalive }) => ({
      chatId: payload.chatId,
      revision: payload.revision,
      keepalive,
    })),
    [
      { chatId: "chat-1", revision: 1, keepalive: false },
      { chatId: "", revision: 2, keepalive: true },
      { chatId: "chat-1", revision: 3, keepalive: false },
    ],
  );
  assert.equal(pushPresenceStore.getState().claimedChatId, "chat-1");

  browser.dispatchWindow("pagehide");
});

test("pagehide withdraws even while the page still reports focus", async (t) => {
  const browser = installBrowser(t);
  const calls = recordPresence(t);
  const { pushPresenceStore } = await loadPresenceStore("pagehide");

  pushPresenceStore.getState().setWatchedChat("chat-1");
  browser.dispatchWindow("pagehide");

  assert.equal(pushPresenceStore.getState().claimedChatId, null);
  assert.equal(pushPresenceStore.getState().onScreenChatId, "chat-1");
  assert.equal(browser.activeIntervals().length, 0);
  assert.equal(calls[1]?.payload.chatId, "");
  assert.equal(calls[1]?.keepalive, true);

  // The forced withdrawal is also idempotent.
  browser.dispatchWindow("pagehide");
  assert.equal(calls.length, 2);
});

test("heartbeat repeats the live claim with increasing revisions", async (t) => {
  const browser = installBrowser(t);
  const calls = recordPresence(t);
  const { pushPresenceStore } = await loadPresenceStore("heartbeat");

  pushPresenceStore.getState().setWatchedChat("chat-1");
  const heartbeat = browser.activeIntervals()[0];
  assert.ok(heartbeat);

  heartbeat.callback();
  heartbeat.callback();

  assert.deepEqual(
    calls.map(({ payload, keepalive }) => ({
      chatId: payload.chatId,
      revision: payload.revision,
      keepalive,
    })),
    [
      { chatId: "chat-1", revision: 1, keepalive: false },
      { chatId: "chat-1", revision: 2, keepalive: false },
      { chatId: "chat-1", revision: 3, keepalive: false },
    ],
  );
  assert.equal(browser.intervals.length, 1);
  assert.equal(pushPresenceStore.getState().revision, 3);

  browser.dispatchWindow("pagehide");
});

test("switching chats replaces the heartbeat and reports the new claim", async (t) => {
  const browser = installBrowser(t);
  const calls = recordPresence(t);
  const { pushPresenceStore } = await loadPresenceStore("switch-chat");

  pushPresenceStore.getState().setWatchedChat("chat-1");
  const firstHeartbeat = browser.activeIntervals()[0];
  pushPresenceStore.getState().setWatchedChat("chat-2");

  assert.equal(firstHeartbeat?.cleared, true);
  assert.equal(browser.intervals.length, 2);
  assert.equal(browser.activeIntervals().length, 1);
  assert.equal(pushPresenceStore.getState().claimedChatId, "chat-2");
  assert.deepEqual(calls.map(({ payload }) => payload.chatId), ["chat-1", "chat-2"]);
  assert.deepEqual(calls.map(({ payload }) => payload.revision), [1, 2]);

  browser.dispatchWindow("pagehide");
});

test("a failed presence request is swallowed without rolling back state or revision", async (t) => {
  const browser = installBrowser(t);
  const calls = recordPresence(t, true);
  const { pushPresenceStore } = await loadPresenceStore("request-failure");

  pushPresenceStore.getState().setWatchedChat("chat-1");
  await new Promise<void>((resolve) => setImmediate(resolve));

  assert.equal(calls.length, 1);
  assert.equal(pushPresenceStore.getState().claimedChatId, "chat-1");
  assert.equal(pushPresenceStore.getState().revision, 1);

  browser.dispatchWindow("pagehide");
  await new Promise<void>((resolve) => setImmediate(resolve));
});

test("outside a browser it remembers the on-screen chat without claiming it", async (t) => {
  removeGlobal(t, "window");
  removeGlobal(t, "document");
  const calls = recordPresence(t);
  const { pushPresenceStore } = await loadPresenceStore("server-render");

  pushPresenceStore.getState().setWatchedChat("chat-1");

  assert.equal(pushPresenceStore.getState().onScreenChatId, "chat-1");
  assert.equal(pushPresenceStore.getState().claimedChatId, null);
  assert.equal(pushPresenceStore.getState().revision, 0);
  assert.deepEqual(calls, []);
});
