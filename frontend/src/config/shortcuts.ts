/** The keyboard chords this app claims, and what each one opens or closes. */

import type { ShortcutChord } from "../models/shortcuts.ts";

/**
 * Cmd on a Mac, Ctrl elsewhere. Both are accepted on both platforms rather
 * than sniffing the OS: a chord the user's other tools bind under either
 * modifier should reach us under either modifier.
 *
 * Alt is never part of one of our chords. It is what the OS layers alternate
 * characters and its own shortcuts onto, so a chord holding it is not ours.
 */
function hasCommandModifier(chord: ShortcutChord): boolean {
  return (chord.metaKey || chord.ctrlKey) && !chord.altKey;
}

/**
 * Cmd/Ctrl+P (VS Code's quick open) or Cmd/Ctrl+K opens the search palette.
 *
 * Cmd/Ctrl+Shift+P is deliberately left alone: it is the conventional "command
 * palette" chord, and swallowing it would break the browser's handling of a
 * shortcut this app does not implement.
 */
export function isPaletteShortcut(chord: ShortcutChord): boolean {
  if (!hasCommandModifier(chord)) return false;
  const key = chord.key.toLowerCase();
  if (key === "k") return true;
  return key === "p" && !chord.shiftKey;
}

/**
 * Cmd/Ctrl+F opens find-in-chat.
 *
 * It deliberately takes the browser's own find, because the two would otherwise
 * compete over the same thread: the native one cannot reach messages the list
 * has not rendered, and it searches the sidebar and composer alongside them.
 */
export function isFindShortcut(chord: ShortcutChord): boolean {
  return hasCommandModifier(chord) && !chord.shiftKey && chord.key.toLowerCase() === "f";
}

/**
 * Escape closes whatever transient surface is on top: find-in-chat, the
 * palette, a menu, a modal, the mobile sidebar, or a streaming reply.
 *
 * Which one it reaches is a matter of listener order rather than of this
 * predicate -- see `useShortcut`'s capture option. What is decided here is only
 * that a bare Escape is ours and a modified one is not: Shift+Escape opens the
 * browser's task manager, and the Cmd/Ctrl/Alt combinations belong to the OS.
 * An Escape that ends an IME composition is the composer's, not ours.
 */
export function isDismissShortcut(chord: ShortcutChord): boolean {
  if (chord.isComposing) return false;
  return (
    chord.key === "Escape" &&
    !chord.metaKey &&
    !chord.ctrlKey &&
    !chord.altKey &&
    !chord.shiftKey
  );
}
