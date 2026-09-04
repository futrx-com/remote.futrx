// Escape, bound for one dismissible surface.
//
// Find-in-chat, the menus, the modals, the mobile sidebar, and a streaming
// reply all close on Escape, and each was pairing `useShortcut` with
// `isDismissShortcut` itself. Naming the pairing says the intent once.
//
// Which of them a press reaches is `dismissStackService`'s rule, not this
// hook's: every open surface listens, and the one holding the dismissal acts.
// The hook keeps the lifecycle -- take a place on screen, give it back on the
// way out -- which is the half a test cannot reach.

import { useEffect, useRef } from "preact/hooks";
import { isDismissShortcut } from "../../../config/shortcuts.ts";
import {
  NO_DISMISS_CLAIM,
  dismissStackService,
} from "../../../services/platform/dismissStackService.ts";
import { useShortcut } from "./useShortcut.ts";

interface DismissOptions {
  /** Listen only while the surface is on screen; defaults to always. */
  enabled?: boolean;
  /**
   * This is not a surface, but something Escape falls through to once every
   * surface is closed -- the streaming reply it cancels.
   */
  fallback?: boolean;
}

/**
 * Dismiss on Escape, if this surface is the one in front.
 *
 * A handler on the focused element still comes first: the palette, the search
 * box, and a tooltip each stop the key on their own node, so they answer before
 * any of this runs.
 *
 * The event is handed on, because whether Escape also has a browser default
 * worth suppressing depends on what the surface has focused -- a text input
 * behaves differently from a menu -- and is not a property of the chord.
 */
export function useDismissShortcut(
  onDismiss: (event: KeyboardEvent) => void,
  { enabled = true, fallback = false }: DismissOptions = {}
): void {
  const claimRef = useRef(NO_DISMISS_CLAIM);

  useEffect(() => {
    if (!enabled) return;
    const claim = dismissStackService.claim({ fallback });
    claimRef.current = claim;
    return () => {
      claimRef.current = NO_DISMISS_CLAIM;
      dismissStackService.release(claim);
    };
  }, [enabled, fallback]);

  useShortcut(
    isDismissShortcut,
    (event) => {
      if (!dismissStackService.owns(claimRef.current)) return;
      onDismiss(event);
    },
    { enabled }
  );
}
