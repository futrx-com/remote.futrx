import type { ComponentChildren } from "preact";

export function AppShell({
  sidebar,
  children,
}: {
  sidebar: ComponentChildren;
  children: ComponentChildren;
}) {
  return (
    <div class="codex-app app-shell flex bg-app text-ink-100 overflow-hidden">
      {sidebar}
      <main class="codex-main codex-window-frame relative flex-1 flex flex-col min-w-0 h-full overflow-hidden bg-canvas">
        {children}
      </main>
    </div>
  );
}
