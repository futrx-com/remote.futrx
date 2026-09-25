import assert from "node:assert/strict";
import test from "node:test";
import { pushServiceWorkerApi } from "../../../api/pushServiceWorkerApi.ts";
import { pushPresenceStore } from "./pushPresenceStore.ts";
import { closeNotificationsOfWatchedChat } from "./pushTrayCleanup.ts";

test("watching a chat closes the notifications it already raised, once per claim", (t) => {
  pushPresenceStore.setState({ claimedChatId: null });
  const closed: string[] = [];
  t.mock.method(pushServiceWorkerApi, "closeChatNotifications", async (chatId: string) => {
    closed.push(chatId);
  });
  const unsubscribe = closeNotificationsOfWatchedChat();
  t.after(unsubscribe);

  pushPresenceStore.setState({ claimedChatId: "chat-1" });
  // A heartbeat revision is not a new claim.
  pushPresenceStore.setState({ revision: 7 });
  // Switching away (blur, background, another view) withdraws the claim...
  pushPresenceStore.setState({ claimedChatId: null });
  // ...and coming back to the same chat counts as reading it again.
  pushPresenceStore.setState({ claimedChatId: "chat-1" });
  pushPresenceStore.setState({ claimedChatId: "chat-2" });

  assert.deepEqual(closed, ["chat-1", "chat-1", "chat-2"]);
});

test("unsubscribing stops closing notifications", (t) => {
  pushPresenceStore.setState({ claimedChatId: null });
  const closed: string[] = [];
  t.mock.method(pushServiceWorkerApi, "closeChatNotifications", async (chatId: string) => {
    closed.push(chatId);
  });

  closeNotificationsOfWatchedChat()();
  pushPresenceStore.setState({ claimedChatId: "chat-1" });

  assert.deepEqual(closed, []);
});

test("a chat already claimed when the cleanup starts is cleared right away", (t) => {
  pushPresenceStore.setState({ claimedChatId: "chat-1" });
  const closed: string[] = [];
  t.mock.method(pushServiceWorkerApi, "closeChatNotifications", async (chatId: string) => {
    closed.push(chatId);
  });

  const unsubscribe = closeNotificationsOfWatchedChat();
  t.after(unsubscribe);

  assert.deepEqual(closed, ["chat-1"]);
});
