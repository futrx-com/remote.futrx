import assert from "node:assert/strict";
import test from "node:test";
import { extensionDrawerState } from "./extensionDrawerState.ts";

const limits = { defaultWidth: 560, minWidth: 420 };

test("a stored width is reused within the drawer's limits", () => {
  assert.equal(extensionDrawerState.storedWidth("640", limits), 640);
  assert.equal(extensionDrawerState.storedWidth("100", limits), 420);
  assert.equal(extensionDrawerState.storedWidth("5000", limits), 1100);
});

test("a missing or unusable stored width falls back to the default", () => {
  for (const stored of [null, "", "wide", "0", "-300", "NaN"]) {
    assert.equal(extensionDrawerState.storedWidth(stored, limits), 560, String(stored));
  }
});

test("the drawer leaves room for the chat but never drops below its minimum", () => {
  assert.equal(extensionDrawerState.availableWidth(1000, 420), 640);
  assert.equal(extensionDrawerState.availableWidth(600, 420), 420);
  assert.equal(extensionDrawerState.availableWidth(4000, 420), 1100);
});

test("a drag is clamped to the available width", () => {
  assert.equal(extensionDrawerState.clampWidth(900, 420, 640), 640);
  assert.equal(extensionDrawerState.clampWidth(200, 420, 640), 420);
  // A container too narrow for the minimum still yields the minimum.
  assert.equal(extensionDrawerState.clampWidth(500, 420, 300), 420);
});

test("each drawer has its own pane name, element id and storage key", () => {
  assert.equal(extensionDrawerState.paneName("terminal-shell"), "extension-terminal-shell");
  assert.equal(
    extensionDrawerState.paneElementId("terminal-shell"),
    "workspace-extension-terminal-shell-pane",
  );
  assert.notEqual(
    extensionDrawerState.widthStorageKey("a-one"),
    extensionDrawerState.widthStorageKey("a-two"),
  );
});
