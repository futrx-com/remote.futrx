import assert from "node:assert/strict";
import test from "node:test";
import { commandPaletteStore } from "./commandPaletteStore.ts";

test.afterEach(() => {
  commandPaletteStore.getState().closePalette();
});

test("opens, toggles and closes the palette", () => {
  commandPaletteStore.getState().openPalette();
  assert.equal(commandPaletteStore.getState().open, true);

  commandPaletteStore.getState().togglePalette();
  assert.equal(commandPaletteStore.getState().open, false);

  commandPaletteStore.getState().togglePalette();
  assert.equal(commandPaletteStore.getState().open, true);

  commandPaletteStore.getState().closePalette();
  assert.equal(commandPaletteStore.getState().open, false);
});

test("repeating a state does not publish a redundant update", () => {
  const before = commandPaletteStore.getState();
  let notifications = 0;
  const unsubscribe = commandPaletteStore.subscribe(() => {
    notifications += 1;
  });

  commandPaletteStore.getState().closePalette();

  assert.equal(commandPaletteStore.getState(), before);
  assert.equal(notifications, 0);
  unsubscribe();
});
