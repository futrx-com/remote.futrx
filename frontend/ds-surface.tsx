// Design-sync surface: components assume the app's thread ground, which the
// theme tokens in src/index.css resolve per theme. Preview cards need the same
// ground for the components to read correctly, so this is the provider wrapper
// for the claude.ai/design bundle.
import type { ComponentChildren } from "preact";

export function DsSurface({ children }: { children?: ComponentChildren }) {
  return (
    <div class="bg-canvas text-ink-100 font-sans antialiased p-6 min-h-full">
      {children}
    </div>
  );
}
