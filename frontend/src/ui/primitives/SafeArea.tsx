import type { ComponentChildren, JSX } from "preact";
import { safeAreaStyle, type SafeAreaEdge } from "./safeAreaInsets";

/**
 * A container that pads its content clear of the device safe-area insets
 * (notch, status bar, home indicator) on the requested edges — the
 * equivalent of React Native's SafeAreaView.
 *
 * Fixed, full-viewport surfaces need this: unlike content inside the app
 * frame, they are positioned against the raw viewport, so without inset
 * padding their edge controls can sit under the device chrome and become
 * unreachable — the media viewer's close button in the PWA, for instance.
 * The surface's own background still paints the inset area; only the
 * content is pushed clear of it.
 *
 * `style` entries win over the inset padding, so a caller can override an
 * edge it positions by other means.
 */
export function SafeArea({
  edges,
  style,
  children,
  ...rest
}: {
  edges?: readonly SafeAreaEdge[];
  style?: JSX.CSSProperties;
  children?: ComponentChildren;
} & Omit<JSX.HTMLAttributes<HTMLDivElement>, "style">) {
  return (
    <div {...rest} style={{ ...safeAreaStyle(edges), ...(style ?? {}) }}>
      {children}
    </div>
  );
}
