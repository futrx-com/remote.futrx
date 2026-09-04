import assert from "node:assert/strict";
import test from "node:test";
import type { ShortcutChord } from "../models/shortcuts.ts";
import { isDismissShortcut, isFindShortcut, isPaletteShortcut } from "./shortcuts.ts";

const chord = (over: Partial<ShortcutChord>): ShortcutChord => ({
  key: "p",
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  isComposing: false,
  ...over,
});

test("the palette opens on Cmd/Ctrl+P and Cmd/Ctrl+K", () => {
  assert.equal(isPaletteShortcut(chord({ metaKey: true })), true);
  assert.equal(isPaletteShortcut(chord({ ctrlKey: true })), true);
  assert.equal(isPaletteShortcut(chord({ key: "K", metaKey: true })), true);
  // A bare key must keep typing "p" into the search box.
  assert.equal(isPaletteShortcut(chord({})), false);
  // Cmd+Shift+P belongs to the browser, not to us.
  assert.equal(isPaletteShortcut(chord({ metaKey: true, shiftKey: true })), false);
  assert.equal(isPaletteShortcut(chord({ metaKey: true, altKey: true })), false);
  assert.equal(isPaletteShortcut(chord({ key: "j", metaKey: true })), false);
});

test("find-in-chat opens on an unmodified Cmd/Ctrl+F", () => {
  assert.equal(isFindShortcut(chord({ key: "f", metaKey: true })), true);
  assert.equal(isFindShortcut(chord({ key: "F", ctrlKey: true })), true);
  // Plain "f" is a character, and the modified chords are the browser's.
  assert.equal(isFindShortcut(chord({ key: "f" })), false);
  assert.equal(isFindShortcut(chord({ key: "f", metaKey: true, shiftKey: true })), false);
  assert.equal(isFindShortcut(chord({ key: "f", metaKey: true, altKey: true })), false);
  assert.equal(isFindShortcut(chord({ key: "g", metaKey: true })), false);
});

test("only a bare Escape dismisses", () => {
  assert.equal(isDismissShortcut(chord({ key: "Escape" })), true);
  // Shift+Escape opens the browser's task manager; the rest are the OS's.
  assert.equal(isDismissShortcut(chord({ key: "Escape", shiftKey: true })), false);
  assert.equal(isDismissShortcut(chord({ key: "Escape", metaKey: true })), false);
  assert.equal(isDismissShortcut(chord({ key: "Escape", ctrlKey: true })), false);
  assert.equal(isDismissShortcut(chord({ key: "Escape", altKey: true })), false);
  // Escape ends an in-flight IME composition rather than closing what is open.
  assert.equal(isDismissShortcut(chord({ key: "Escape", isComposing: true })), false);
  // Escape is spelled exactly, never lowercased the way letter chords are.
  assert.equal(isDismissShortcut(chord({ key: "escape" })), false);
  assert.equal(isDismissShortcut(chord({ key: "Esc" })), false);
});
