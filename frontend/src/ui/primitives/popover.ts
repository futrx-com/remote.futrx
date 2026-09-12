// Shared popover dismissal behaviour.
//
// `ComposerOptionDropdown`, `SkillPicker`, and `WorkspaceActions` each grew
// their own copy of this logic. New menus use these hooks instead of adding a
// fourth; the existing three can be migrated onto them separately.

import { useEffect } from "preact/hooks";
import type { RefObject } from "preact";
import { useDismissShortcut } from "../../state/hooks/shared/useDismissShortcut.ts";

/** Close on an outside pointer press or Escape, while `open`. */
export function useDismissOnOutside(
  open: boolean,
  onClose: () => void,
  rootRef: RefObject<HTMLElement>
): void {
  useEffect(() => {
    if (!open) return;

    function closeOnOutsidePress(event: MouseEvent) {
      const target = event.target as Node | null;
      if (target && !rootRef.current?.contains(target)) onClose();
    }
    window.addEventListener("mousedown", closeOnOutsidePress);
    return () => window.removeEventListener("mousedown", closeOnOutsidePress);
  }, [open, onClose, rootRef]);

  // A menu opens over the drawer holding it, so it takes the Escape that
  // would otherwise close them both.
  useDismissShortcut(onClose, { enabled: open });
}
