// Safe-area edges a full-viewport surface can keep clear of the device
// chrome: the notch / status bar at the top, the home indicator at the
// bottom, and the rounded corners / camera cutouts on the sides in
// landscape. The page opts into `viewport-fit=cover` (see index.html), so
// these insets are real: a `fixed inset-0` surface that ignores them puts
// its edge content — a viewer's close button, say — under the status bar
// where it cannot be tapped.
export type SafeAreaEdge = "top" | "right" | "bottom" | "left";

export const SAFE_AREA_EDGES: readonly SafeAreaEdge[] = [
  "top",
  "right",
  "bottom",
  "left",
];

const PADDING_PROPERTY: Record<SafeAreaEdge, string> = {
  top: "paddingTop",
  right: "paddingRight",
  bottom: "paddingBottom",
  left: "paddingLeft",
};

// The CSS value that resolves to the device inset for one edge, falling
// back to zero on displays and browsers without insets.
export function safeAreaInset(edge: SafeAreaEdge): string {
  return `env(safe-area-inset-${edge}, 0px)`;
}

// Inline padding that keeps a surface's content clear of the requested
// edges. Defaults to every edge; duplicate edges are applied once and an
// empty list applies nothing.
export function safeAreaStyle(
  edges: readonly SafeAreaEdge[] = SAFE_AREA_EDGES,
): Record<string, string> {
  const style: Record<string, string> = {};
  for (const edge of new Set(edges)) {
    style[PADDING_PROPERTY[edge]] = safeAreaInset(edge);
  }
  return style;
}
