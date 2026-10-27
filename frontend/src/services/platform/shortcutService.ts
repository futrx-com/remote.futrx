import type { ShortcutChord } from "../../models/shortcuts.ts";

/** Decide which keyboard chords the app claims; callers own binding and effects. */
class ShortcutService {
  /**
   * Cmd/Ctrl+P or Cmd/Ctrl+K opens search. Shift+P remains available to the
   * browser; Shift+K is accepted alongside K.
   *
   * Bound predicates can be handed directly to a shortcut listener.
   */
  readonly isPalette = (chord: ShortcutChord): boolean => {
    if (!this.#hasCommandModifier(chord)) return false;
    const key = chord.key.toLowerCase();
    if (key === "k") return true;
    return key === "p" && !chord.shiftKey;
  };

  /** Cmd/Ctrl+F opens find-in-chat; the caller claims the browser's find. */
  readonly isFind = (chord: ShortcutChord): boolean => {
    return this.#hasCommandModifier(chord) && !chord.shiftKey && chord.key.toLowerCase() === "f";
  };

  /**
   * Only bare Escape dismisses a surface. Modified Escape belongs to the
   * browser or OS, and an Escape ending IME composition belongs to the input.
   * Dismissal order is owned by `dismissStackService`.
   */
  readonly isDismiss = (chord: ShortcutChord): boolean => {
    if (chord.isComposing) return false;
    return (
      chord.key === "Escape" &&
      !chord.metaKey &&
      !chord.ctrlKey &&
      !chord.altKey &&
      !chord.shiftKey
    );
  };

  /** Accept either platform's command modifier, but never Alt. */
  #hasCommandModifier(chord: ShortcutChord): boolean {
    return (chord.metaKey || chord.ctrlKey) && !chord.altKey;
  }
}

export const shortcutService = new ShortcutService();
