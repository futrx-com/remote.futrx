// The shape a keyboard shortcut decision is made from.
//
// It lives apart from any one feature's model because the same chords are read
// by search, find-in-chat, the sidebar, and every dismissible overlay. The
// predicates that interpret it are in `config/shortcuts.ts`; the binding that
// listens for it is `state/hooks/shared/useShortcut.ts`.

/**
 * The subset of a keyboard event a shortcut decision depends on.
 *
 * A `KeyboardEvent` satisfies this structurally, so handlers pass the event
 * straight through, and tests build a chord literal without a DOM.
 */
export interface ShortcutChord {
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
  /** True while an IME is mid-composition, when keys belong to the composer. */
  isComposing: boolean;
}
