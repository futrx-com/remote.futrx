// How tall the app shell should be, given what the browser reports about the
// viewport. Only an open keyboard justifies overriding the `100dvh` the
// stylesheet already asks for: under `viewport-fit=cover` iOS reports both
// `innerHeight` and `visualViewport.height` short of the layout viewport by the
// top safe-area inset, so following them unconditionally leaves a strip of bare
// page background below the composer.
//
// Leaf module: it knows the arithmetic, never how the numbers were measured.

import { KEYBOARD_MIN_COVERAGE_PX } from "../../config/viewport.ts";

interface ViewportMetrics {
  /** What `100dvh` resolves to: the full layout viewport, insets included. */
  layoutHeight: number;
  /** Absent on browsers without a visual viewport, which is why it is one
   *  nullable value rather than two fields that could disagree. */
  visual: { height: number; offsetTop: number } | null;
  inputFocused: boolean;
}

interface ViewportOverride {
  height: number;
  offsetTop: number;
}

class ViewportHeightService {
  /** The height and offset to pin the shell to, or null to leave `100dvh` alone. */
  keyboardOverride(metrics: ViewportMetrics): ViewportOverride | null {
    const { visual } = metrics;
    if (!metrics.inputFocused || !visual) return null;
    if (metrics.layoutHeight - visual.height <= KEYBOARD_MIN_COVERAGE_PX) return null;
    return { height: Math.round(visual.height), offsetTop: Math.round(visual.offsetTop) };
  }
}

export const viewportHeightService = new ViewportHeightService();
