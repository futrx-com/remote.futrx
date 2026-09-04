import assert from "node:assert/strict";
import test from "node:test";
import type { ShortcutChord } from "../../models/shortcuts.ts";
import { commandPaletteKeyState } from "./commandPaletteKeyState.ts";

const press = (key: string, over: Partial<ShortcutChord> = {}): ShortcutChord => ({
  key,
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  isComposing: false,
  ...over,
});

const at = (activeIndex: number, resultCount: number, filtersOpen = false) => ({
  activeIndex,
  resultCount,
  filtersOpen,
});

test("the arrows walk the results", () => {
  assert.deepEqual(commandPaletteKeyState.next(press("ArrowDown"), at(0, 3)), {
    kind: "highlight",
    index: 1,
  });
  assert.deepEqual(commandPaletteKeyState.next(press("ArrowUp"), at(2, 3)), {
    kind: "highlight",
    index: 1,
  });
});

test("the arrows wrap at both ends", () => {
  assert.deepEqual(commandPaletteKeyState.next(press("ArrowDown"), at(2, 3)), {
    kind: "highlight",
    index: 0,
  });
  assert.deepEqual(commandPaletteKeyState.next(press("ArrowUp"), at(0, 3)), {
    kind: "highlight",
    index: 2,
  });
});

test("the arrows stay put on an empty list rather than dividing by zero", () => {
  for (const key of ["ArrowDown", "ArrowUp"]) {
    const action = commandPaletteKeyState.next(press(key), at(0, 0));
    assert.deepEqual(action, { kind: "highlight", index: 0 });
  }
});

test("Home and End jump to the ends, and hold at zero on an empty list", () => {
  assert.deepEqual(commandPaletteKeyState.next(press("Home"), at(2, 3)), {
    kind: "highlight",
    index: 0,
  });
  assert.deepEqual(commandPaletteKeyState.next(press("End"), at(0, 3)), {
    kind: "highlight",
    index: 2,
  });
  assert.deepEqual(commandPaletteKeyState.next(press("End"), at(0, 0)), {
    kind: "highlight",
    index: 0,
  });
});

test("Enter opens the highlighted row", () => {
  assert.deepEqual(commandPaletteKeyState.next(press("Enter"), at(1, 3)), { kind: "open" });
});

test("Enter on an empty list does nothing", () => {
  // Otherwise a query still being typed would close the palette out from
  // under whoever typed it.
  assert.deepEqual(commandPaletteKeyState.next(press("Enter"), at(0, 0)), { kind: "ignore" });
});

test("Escape closes the palette", () => {
  assert.deepEqual(commandPaletteKeyState.next(press("Escape"), at(0, 3)), { kind: "close" });
});

test("Escape steps back out of the filter menu instead of closing the palette", () => {
  assert.deepEqual(commandPaletteKeyState.next(press("Escape"), at(0, 3, true)), {
    kind: "closeFilters",
  });
});

test("the filter menu owns the arrows and Enter while it is up", () => {
  for (const key of ["ArrowDown", "ArrowUp", "Home", "End", "Enter"]) {
    assert.deepEqual(
      commandPaletteKeyState.next(press(key), at(0, 3, true)),
      { kind: "ignore" },
      `${key} belongs to the filter menu`
    );
  }
});

test("a modified Escape is left to the browser and the OS", () => {
  for (const modifier of ["shiftKey", "metaKey", "ctrlKey", "altKey"] as const) {
    assert.deepEqual(
      commandPaletteKeyState.next(press("Escape", { [modifier]: true }), at(0, 3)),
      { kind: "ignore" },
      `Escape+${modifier} is not ours`
    );
    assert.deepEqual(
      commandPaletteKeyState.next(press("Escape", { [modifier]: true }), at(0, 3, true)),
      { kind: "ignore" },
      `Escape+${modifier} is not ours with the filter menu up`
    );
  }
});

test("an Escape ending an IME composition belongs to the composer", () => {
  assert.deepEqual(
    commandPaletteKeyState.next(press("Escape", { isComposing: true }), at(0, 3)),
    { kind: "ignore" }
  );
});

test("a character key is left alone so it reaches the query box", () => {
  for (const key of ["a", " ", "Tab", "PageDown"]) {
    assert.deepEqual(
      commandPaletteKeyState.next(press(key), at(0, 3)),
      { kind: "ignore" },
      `${key} is the input's`
    );
  }
});
