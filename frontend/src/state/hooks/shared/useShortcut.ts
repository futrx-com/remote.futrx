// The one place a keyboard shortcut is bound to the window.
//
// Cmd/Ctrl+P, Cmd/Ctrl+F, and the Escape that closes each overlay had each
// grown their own copy of the same effect: add a `keydown` listener, compare
// `event.key` inline, call the handler, remove the listener. Every copy was a
// chance to spell a chord differently from `config/shortcuts.ts`, and several
// needed a hand-rolled ref so the listener could see current state without
// re-registering. Both problems are solved once here.

import { useEffect, useRef } from "preact/hooks";
import type { ShortcutChord } from "../../../models/shortcuts.ts";

interface ShortcutOptions {
  /** Listen only while true; defaults to always. */
  enabled?: boolean;
}

/**
 * Run `onMatch` whenever a `keydown` on the window matches `matches`.
 *
 * `matches` is one of the predicates from `config/shortcuts.ts`, so the chord
 * itself is described in exactly one place. Neither callback needs to be
 * stable: both are read through a ref, so an inline arrow closing over current
 * state is correct here and does not re-register the listener.
 *
 * The event is handed to `onMatch` rather than pre-empted, because claiming a
 * chord from the browser (`preventDefault`) is the caller's decision, not a
 * property of the chord.
 *
 * Two surfaces wanting the same chord is not settled here -- listener order
 * cannot express which is in front. See `dismissStackService`.
 */
export function useShortcut(
  matches: (chord: ShortcutChord) => boolean,
  onMatch: (event: KeyboardEvent) => void,
  { enabled = true }: ShortcutOptions = {}
): void {
  const matchesRef = useRef(matches);
  matchesRef.current = matches;
  const onMatchRef = useRef(onMatch);
  onMatchRef.current = onMatch;

  useEffect(() => {
    if (!enabled) return;
    function onKeyDown(event: KeyboardEvent) {
      if (matchesRef.current(event)) onMatchRef.current(event);
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [enabled]);
}
