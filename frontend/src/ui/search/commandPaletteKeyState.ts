// What a key does in the command palette.
//
// The rule was a switch inside the component, where nothing could reach it:
// the wrap-around at both ends of the list, the empty-list case that must not
// divide by zero, and the filter menu's claim on the arrows are all decisions
// worth pinning, and none of them needs a palette on screen to be true.
//
// The component keeps what a test cannot have -- the focus moves, the store
// writes, the scroll into view -- and reads its next step from here.

import { isDismissShortcut } from "../../config/shortcuts.ts";
import type { CommandPaletteKeyAction } from "../../models/search";
import type { ShortcutChord } from "../../models/shortcuts.ts";

const IGNORE: CommandPaletteKeyAction = { kind: "ignore" };

class CommandPaletteKeyState {
  /**
   * What `chord` does, given where the cursor is and how many rows there are.
   *
   * While the filter menu is up it owns the arrows and Enter, and Escape steps
   * back to the results rather than closing the palette outright -- one press,
   * one layer.
   */
  next(
    chord: ShortcutChord,
    { activeIndex, resultCount, filtersOpen }: {
      activeIndex: number;
      resultCount: number;
      filtersOpen: boolean;
    }
  ): CommandPaletteKeyAction {
    if (filtersOpen) return isDismissShortcut(chord) ? { kind: "closeFilters" } : IGNORE;
    if (isDismissShortcut(chord)) return { kind: "close" };

    switch (chord.key) {
      case "ArrowDown":
        return this.#step(activeIndex, resultCount, 1);
      case "ArrowUp":
        return this.#step(activeIndex, resultCount, -1);
      case "Home":
        return { kind: "highlight", index: 0 };
      case "End":
        return { kind: "highlight", index: Math.max(0, resultCount - 1) };
      case "Enter":
        // An empty list has nothing to open, and Enter on it must not close
        // the palette out from under a query still being typed.
        return resultCount > 0 ? { kind: "open" } : IGNORE;
      default:
        return IGNORE;
    }
  }

  /** Wraps at both ends, and stays put on an empty list rather than dividing by zero. */
  #step(activeIndex: number, resultCount: number, delta: number): CommandPaletteKeyAction {
    if (resultCount === 0) return { kind: "highlight", index: 0 };
    return { kind: "highlight", index: (activeIndex + delta + resultCount) % resultCount };
  }
}

export const commandPaletteKeyState = new CommandPaletteKeyState();
