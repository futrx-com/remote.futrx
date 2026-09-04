import assert from "node:assert/strict";
import test from "node:test";
import { pushServiceWorkerApi } from "../../../api/pushServiceWorkerApi.ts";
import { pushNotificationStore } from "./pushNotificationStore.ts";

type PushCallbacks = Parameters<typeof pushServiceWorkerApi.connect>[0];

function resetStore(): void {
  pushNotificationStore.getState().setVisibleChat(null);
}

test("starts without a visible chat", () => {
  resetStore();
  assert.equal(pushNotificationStore.getState().visibleChatId, null);
});

test("setVisibleChat publishes the current chat and can clear it", () => {
  resetStore();
  const seen: Array<string | null> = [];
  const unsubscribe = pushNotificationStore.subscribe((state) => {
    seen.push(state.visibleChatId);
  });

  pushNotificationStore.getState().setVisibleChat("chat-1");
  pushNotificationStore.getState().setVisibleChat(null);
  unsubscribe();

  assert.deepEqual(seen, ["chat-1", null]);
  assert.equal(pushNotificationStore.getState().visibleChatId, null);
});

test("connect registers first and routes notification taps to the supplied opener", (t) => {
  resetStore();
  const sequence: string[] = [];
  let callbacks: PushCallbacks | undefined;
  const register = t.mock.method(pushServiceWorkerApi, "register", async () => {
    sequence.push("register");
    return null;
  });
  const connect = t.mock.method(pushServiceWorkerApi, "connect", (nextCallbacks: PushCallbacks) => {
    sequence.push("connect");
    callbacks = nextCallbacks;
  });
  const opened: Array<string | null> = [];

  pushNotificationStore.getState().connect((chatId) => opened.push(chatId));

  assert.deepEqual(sequence, ["register", "connect"]);
  assert.equal(register.mock.callCount(), 1);
  assert.equal(connect.mock.callCount(), 1);
  assert.ok(callbacks);
  callbacks.openChat("chat-2");
  callbacks.openChat(null);
  assert.deepEqual(opened, ["chat-2", null]);
});

test("reports the visible chat only while the page is visible and focused", (t) => {
  resetStore();
  let visibilityState: DocumentVisibilityState = "visible";
  let focused = true;
  const previousDocument = Object.getOwnPropertyDescriptor(globalThis, "document");
  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: {
      get visibilityState() {
        return visibilityState;
      },
      hasFocus: () => focused,
    },
  });
  t.after(() => {
    if (previousDocument) {
      Object.defineProperty(globalThis, "document", previousDocument);
    } else {
      Reflect.deleteProperty(globalThis, "document");
    }
  });

  let callbacks: PushCallbacks | undefined;
  t.mock.method(pushServiceWorkerApi, "register", async () => null);
  t.mock.method(pushServiceWorkerApi, "connect", (nextCallbacks: PushCallbacks) => {
    callbacks = nextCallbacks;
  });

  pushNotificationStore.getState().setVisibleChat("chat-1");
  pushNotificationStore.getState().connect(() => {});
  assert.ok(callbacks);
  assert.equal(callbacks.visibleChatId(), "chat-1");

  visibilityState = "hidden";
  assert.equal(callbacks.visibleChatId(), null);

  visibilityState = "visible";
  focused = false;
  assert.equal(callbacks.visibleChatId(), null);

  focused = true;
  pushNotificationStore.getState().setVisibleChat("chat-2");
  assert.equal(callbacks.visibleChatId(), "chat-2");

  pushNotificationStore.getState().setVisibleChat(null);
  assert.equal(callbacks.visibleChatId(), null);
});
