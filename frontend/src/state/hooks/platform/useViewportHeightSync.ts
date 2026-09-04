import { useLayoutEffect } from "preact/hooks";
import { VIEWPORT_FOCUS_SETTLE_DELAY_MS } from "../../../config/viewport.ts";
import { viewportHeightService } from "../../../services/platform/viewportHeightService.ts";

/** Keeps the app-shell CSS variables aligned with a keyboard-covered viewport. */
export function useViewportHeightSync(): void {
  useLayoutEffect(() => {
    let frame = 0;
    const focusTimers = new Set<number>();
    const observedVisualViewport = window.visualViewport;

    const inputFocused = () => {
      const active = document.activeElement;
      const tag = active?.tagName.toLowerCase();
      return (
        tag === "input" ||
        tag === "textarea" ||
        active?.getAttribute("contenteditable") === "true"
      );
    };

    const sync = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(() => {
        const html = document.documentElement;
        const visualViewport = window.visualViewport;
        const override = viewportHeightService.keyboardOverride({
          layoutHeight: html.clientHeight,
          visual: visualViewport
            ? { height: visualViewport.height, offsetTop: visualViewport.offsetTop }
            : null,
          inputFocused: inputFocused(),
        });

        if (!override) {
          html.style.removeProperty("--app-height");
          html.style.removeProperty("--app-offset-top");
          return;
        }
        html.style.setProperty("--app-height", `${override.height}px`);
        html.style.setProperty("--app-offset-top", `${override.offsetTop}px`);
      });
    };

    const syncAfterFocusSettles = () => {
      const timer = window.setTimeout(() => {
        focusTimers.delete(timer);
        sync();
      }, VIEWPORT_FOCUS_SETTLE_DELAY_MS);
      focusTimers.add(timer);
    };

    sync();
    window.addEventListener("resize", sync);
    window.addEventListener("orientationchange", sync);
    window.addEventListener("focusin", sync);
    window.addEventListener("focusout", syncAfterFocusSettles);
    observedVisualViewport?.addEventListener("resize", sync);
    observedVisualViewport?.addEventListener("scroll", sync);

    return () => {
      cancelAnimationFrame(frame);
      for (const timer of focusTimers) window.clearTimeout(timer);
      document.documentElement.style.removeProperty("--app-height");
      document.documentElement.style.removeProperty("--app-offset-top");
      window.removeEventListener("resize", sync);
      window.removeEventListener("orientationchange", sync);
      window.removeEventListener("focusin", sync);
      window.removeEventListener("focusout", syncAfterFocusSettles);
      observedVisualViewport?.removeEventListener("resize", sync);
      observedVisualViewport?.removeEventListener("scroll", sync);
    };
  }, []);
}
