import assert from "node:assert/strict";
import test from "node:test";
import type { ChatStatus } from "../../../models/chat";
import type { ConfirmFn, ConfirmOptions } from "../../context/ConfirmContext";
import { runChatRewind } from "./chatRewindFlow.ts";

function setup(status: ChatStatus = "ready") {
  const events: string[] = [];
  const dialogs: ConfirmOptions[] = [];
  const confirm: ConfirmFn = async (options) => {
    dialogs.push(options);
    events.push(`dialog: ${options.title}`);
    if (options.action) await options.action();
    return true;
  };
  const options = {
    status,
    beforeT: 123,
    promptText: "original prompt",
    confirm,
    rewind: async (t: number) => { events.push(`rewind: ${t}`); },
    clearQueuedPrompts: () => { events.push("clear queue"); },
    restorePrompt: (text: string) => { events.push(`draft: ${text}`); },
    refreshMeta: async () => { events.push("refresh metadata"); },
    onComplete: () => { events.push("focus composer"); },
  };
  return { events, dialogs, options };
}

test("shows a styled informational dialog without rewinding during an active run", async () => {
  const { events, dialogs, options } = setup("streaming");
  await runChatRewind(options);
  assert.deepEqual(events, ["dialog: Rewind unavailable"]);
  assert.equal(dialogs[0].tone, "neutral");
  assert.match(String(dialogs[0].message), /Cancel the current run/);
});

test("canceling the confirmation leaves the draft, queue, and history untouched", async () => {
  const { events, options } = setup();
  options.confirm = async () => false;
  await runChatRewind(options);
  assert.deepEqual(events, []);
});

test("a successful rewind is performed inside the pending dialog before local state changes", async () => {
  const { events, dialogs, options } = setup();
  await runChatRewind(options);
  assert.equal(dialogs[0].pendingLabel, "Rewinding…");
  assert.deepEqual(events, [
    "dialog: Rewind chat",
    "rewind: 123",
    "clear queue",
    "draft: original prompt",
    "refresh metadata",
    "focus composer",
  ]);
});

test("an API failure stays in the confirmation UI and never clears local state", async () => {
  const { events, dialogs, options } = setup();
  options.rewind = async () => { events.push("rewind failed"); throw new Error("upstream unavailable"); };
  options.confirm = async (dialog) => {
    dialogs.push(dialog);
    try { await dialog.action?.(); } catch (error) {
      assert.match(String(error), /upstream unavailable/);
      return false; // User dismisses the still-open shared confirmation dialog.
    }
    return true;
  };
  await runChatRewind(options);
  assert.deepEqual(events, ["rewind failed"]);
  assert.equal(dialogs[0].title, "Rewind chat");
});

test("the failed pending action can be retried without re-confirming or losing the draft", async () => {
  const { events, options } = setup();
  let attempts = 0;
  options.rewind = async () => {
    attempts += 1;
    if (attempts === 1) throw new Error("temporary failure");
    events.push("rewind success");
  };
  options.confirm = async (dialog) => {
    await assert.rejects(dialog.action!(), /temporary failure/);
    assert.deepEqual(events, []);
    await dialog.action!();
    return true;
  };
  await runChatRewind(options);
  assert.equal(attempts, 2);
  assert.deepEqual(events, ["rewind success", "clear queue", "draft: original prompt", "refresh metadata", "focus composer"]);
});

test("metadata refresh failure is explained without re-running the destructive rewind", async () => {
  const { events, dialogs, options } = setup();
  options.refreshMeta = async () => { events.push("refresh failed"); throw new Error("network disconnected"); };
  await runChatRewind(options);
  assert.deepEqual(events, [
    "dialog: Rewind chat", "rewind: 123", "clear queue", "draft: original prompt",
    "refresh failed", "dialog: Chat refresh failed", "focus composer",
  ]);
  assert.equal(dialogs[1].tone, "neutral");
  assert.equal(dialogs[1].message, "network disconnected");
  assert.equal(dialogs[1].action, undefined);
});

test("refresh failures with no error message still give the user a recovery hint", async () => {
  const { dialogs, options } = setup();
  options.refreshMeta = async () => { throw "unknown error"; };
  await runChatRewind(options);
  assert.match(String(dialogs[1].message), /Reload the chat/);
});

test("rewind opts into metadata errors so the live hook can report refresh failures", async () => {
  const { dialogs, options } = setup();
  let throwOnErrorRequested = false;
  options.refreshMeta = async (throwOnError = false) => {
    throwOnErrorRequested = throwOnError;
    if (throwOnError) throw new Error("metadata unavailable");
  };
  await runChatRewind(options);
  assert.equal(throwOnErrorRequested, true);
  assert.equal(dialogs[1].title, "Chat refresh failed");
  assert.equal(dialogs[1].message, "metadata unavailable");
});
