import { useEffect, useRef } from "preact/hooks";
import type {
  ExtensionWorkspacePaneContext,
  ExtensionWorkspacePaneContribution,
} from "../../../models/extension";
import { X } from "../../primitives/icons";

export function ExtensionWorkspacePane({
  pane,
  open,
  context,
  onClose,
}: {
  pane: ExtensionWorkspacePaneContribution;
  open: boolean;
  context: ExtensionWorkspacePaneContext;
  onClose: () => void;
}) {
  const hostRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const host = hostRef.current;
    if (!open || !host) return;
    let cleanup: void | (() => void);
    try {
      cleanup = pane.render(host, context);
    } catch (error) {
      console.error(
        `[extensions] ${pane.applicationId}: workspace pane render failed`,
        error,
      );
    }
    return () => {
      try {
        if (typeof cleanup === "function") cleanup();
      } catch (error) {
        console.error(
          `[extensions] ${pane.applicationId}: workspace pane cleanup failed`,
          error,
        );
      }
      host.replaceChildren();
    };
  }, [pane, open, context]);

  const action = extensionPaneAction(pane.id);

  return (
    <aside
      id={`workspace-${action}-pane`}
      class={`workspace-pane workspace-extension-pane relative z-20 h-full flex-none overflow-hidden bg-surface border-l border-line shadow-2xl transition-[width,opacity] duration-200 ease-out ${
        open ? "opacity-100" : "opacity-0 border-l-0 pointer-events-none"
      }`}
      style={`--workspace-extension-width: ${pane.width ?? 560}px`}
      aria-hidden={!open}
      aria-label={pane.label}
    >
      <div class={`flex h-full min-h-0 w-full flex-col transition-transform duration-200 ease-out ${
        open ? "translate-x-0" : "translate-x-full"
      }`}>
        <header class="workspace-pane-header codex-header flex flex-none items-center gap-2 border-b border-line bg-surface px-3 pb-2.5 md:px-4">
          <span
            class="grid h-9 w-9 flex-none place-items-center rounded-md border border-line bg-tint text-accent-blue [&>svg]:h-4 [&>svg]:w-4"
            aria-hidden="true"
            dangerouslySetInnerHTML={{ __html: pane.icon }}
          />
          <h2 class="min-w-0 flex-1 truncate text-[15px] font-semibold text-ink-50 md:text-base">
            {pane.label}
          </h2>
          <button
            type="button"
            onClick={onClose}
            class="grid h-9 w-9 place-items-center rounded-md border border-line bg-tint text-ink-200 hover:bg-tint-strong"
            title={`Close ${pane.label}`}
            aria-label={`Close ${pane.label}`}
            data-workspace-pane-close
          >
            <X class="h-4 w-4" />
          </button>
        </header>
        {open && <div ref={hostRef} class="min-h-0 flex-1 overflow-hidden" />}
      </div>
    </aside>
  );
}

export function extensionPaneAction(id: string): string {
  return `extension-${id.replace(/[^a-zA-Z0-9_-]/g, "-")}`;
}
