// Design-sync surface: the app renders on the ink-900 dark ground
// (src/index.css sets html/body to #0f1014). Preview cards and designs
// need the same ground for the components to read correctly, so this is
// the provider wrapper for the claude.ai/design bundle.
import type { ComponentChildren } from "preact";

export function DsSurface({ children }: { children?: ComponentChildren }) {
  return (
    <div class="bg-ink-900 text-ink-100 font-sans antialiased p-6 min-h-full">
      {children}
    </div>
  );
}
