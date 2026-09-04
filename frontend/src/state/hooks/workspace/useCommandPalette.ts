import { useStore } from "zustand";
import { isPaletteShortcut } from "../../../config/shortcuts.ts";
import { commandPaletteStore } from "../../stores/workspace/commandPaletteStore.ts";
import { useShortcut } from "../shared/useShortcut.ts";

/**
 * Whether the palette is on screen, plus the chord that toggles it.
 *
 * Call this once, from whatever renders the palette: the shortcut is bound per
 * call, so a second caller would toggle the same flag twice per press and leave
 * it exactly where it was. Surfaces that only need to open it take
 * `useOpenCommandPalette` instead.
 */
export function useCommandPalette(): { open: boolean; close: () => void } {
  const open = useStore(commandPaletteStore, (state) => state.open);
  const close = useStore(commandPaletteStore, (state) => state.closePalette);
  const toggle = useStore(commandPaletteStore, (state) => state.togglePalette);

  useShortcut(isPaletteShortcut, (event) => {
    // Browsers map Cmd/Ctrl+P to Print; preventDefault suppresses it.
    event.preventDefault();
    toggle();
  });

  return { open, close };
}

/**
 * Opening the palette, without subscribing to whether it is open. The action is
 * created once with the store, so a holder never re-renders for it.
 */
export function useOpenCommandPalette(): () => void {
  return useStore(commandPaletteStore, (state) => state.openPalette);
}
