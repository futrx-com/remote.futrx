import { useCallback } from "preact/hooks";
import { shortcutService } from "../../../services/platform/shortcutService.ts";

/**
 * Bind bare Escape to an element's keydown handler. The caller decides what
 * closes and whether the event keeps bubbling to enclosing surfaces.
 */
export function useDismissKeyDown(
  onDismiss: (event: KeyboardEvent) => void
): (event: KeyboardEvent) => void {
  return useCallback((event: KeyboardEvent) => {
    if (shortcutService.isDismiss(event)) onDismiss(event);
  }, [onDismiss]);
}
