// Hosts one drawer an application registered with `remote.ui.registerDrawer`.
// The pane, its header and its resize handle belong to the SPA; the application
// only draws into the body, with DOM APIs, as it does in a slot.

import { useEffect, useRef, useState } from "preact/hooks";
import type {
  ExtensionDrawerContribution,
  ExtensionDrawerStatus,
  ExtensionDrawerTarget,
} from "../../../models/extension";
import { EXTENSION_DRAWER_WIDTH } from "../../../config/extensions";
import { extensionDrawerState } from "../../../state/hooks/chat/extensionDrawerState";
import { X } from "../../primitives/icons";
import { ExtensionDrawerResizeHandle } from "./ExtensionDrawerResizeHandle";

export function ExtensionDrawer({
  drawer,
  target,
  open,
  onClose,
}: {
  drawer: ExtensionDrawerContribution;
  target: ExtensionDrawerTarget;
  open: boolean;
  onClose: () => void;
}) {
  const { chatId, projectId, cwd } = target;
  // Keep the body mounted for this chat once it has been opened, so whatever
  // the application is running in it survives closing and reopening the pane.
  // It is only torn down when the chat changes.
  const [openedChatId, setOpenedChatId] = useState<string | null>(() => (open ? chatId : null));
  const [status, setStatus] = useState<ExtensionDrawerStatus | null>(null);
  const [width, setWidth] = useState(() =>
    extensionDrawerState.storedWidth(
      window.localStorage.getItem(extensionDrawerState.widthStorageKey(drawer.id)),
      drawer,
    ));
  const [resizing, setResizing] = useState(false);
  const asideRef = useRef<HTMLElement>(null);
  const bodyRef = useRef<HTMLDivElement>(null);
  const mounted = openedChatId === chatId;

  useEffect(() => {
    if (open) {
      setOpenedChatId(chatId);
      return;
    }
    setOpenedChatId((current) => (current === chatId ? current : null));
  }, [chatId, open]);

  useEffect(() => {
    const body = bodyRef.current;
    if (!mounted || !body) return;
    let disposed = false;
    let cleanup: void | (() => void);
    // An application that throws while mounting loses its own body and
    // nothing else: the pane and the chat beside it must still paint.
    try {
      cleanup = drawer.mount(body, {
        chatId,
        projectId,
        cwd,
        setStatus: (next) => {
          if (!disposed) setStatus(next ? { label: next.label, active: next.active } : null);
        },
      });
    } catch (error) {
      console.error("[extensions] drawer mount failed", drawer.id, error);
    }
    return () => {
      disposed = true;
      try {
        if (typeof cleanup === "function") cleanup();
      } catch (error) {
        console.error("[extensions] drawer cleanup failed", drawer.id, error);
      }
      body.replaceChildren();
      setStatus(null);
    };
  }, [drawer, mounted, chatId, projectId, cwd]);

  useEffect(() => {
    window.localStorage.setItem(extensionDrawerState.widthStorageKey(drawer.id), String(width));
  }, [drawer.id, width]);

  // Keep the pane within the container as the viewport changes, always leaving
  // room for the chat beside it.
  useEffect(() => {
    if (!open) return;
    function clampToContainer() {
      const bounds = asideRef.current?.parentElement?.getBoundingClientRect();
      if (!bounds) return;
      const available = extensionDrawerState.availableWidth(bounds.width, drawer.minWidth);
      setWidth((current) => extensionDrawerState.clampWidth(current, drawer.minWidth, available));
    }
    clampToContainer();
    window.addEventListener("resize", clampToContainer);
    return () => window.removeEventListener("resize", clampToContainer);
  }, [open, drawer.minWidth]);

  function handleResizeStart(event: PointerEvent) {
    if (event.button !== 0) return;
    event.preventDefault();
    setResizing(true);

    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";

    function finishResize() {
      setResizing(false);
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
      window.removeEventListener("pointermove", resize);
      window.removeEventListener("pointerup", finishResize);
      window.removeEventListener("pointercancel", finishResize);
    }

    function resize(moveEvent: PointerEvent) {
      const bounds = asideRef.current?.parentElement?.getBoundingClientRect();
      if (!bounds) return;
      const available = extensionDrawerState.availableWidth(bounds.width, drawer.minWidth);
      // Dragging the left edge: width grows as the pointer moves left.
      const next = bounds.right - moveEvent.clientX;
      setWidth(extensionDrawerState.clampWidth(next, drawer.minWidth, available));
    }

    window.addEventListener("pointermove", resize, { passive: false });
    window.addEventListener("pointerup", finishResize);
    window.addEventListener("pointercancel", finishResize);
  }

  return (
    <aside
      ref={asideRef}
      id={extensionDrawerState.paneElementId(drawer.id)}
      class={`workspace-pane workspace-extension-pane relative z-20 h-full flex-none overflow-hidden bg-surface border-l border-line
              ${resizing ? "transition-none" : "transition-[width,opacity] duration-200 ease-out"}
              ${open ? "opacity-100 shadow-2xl" : "opacity-0 border-l-0 shadow-none pointer-events-none"}`}
      style={`--workspace-extension-width: ${width}px; --workspace-extension-max-width: max(${drawer.minWidth}px, calc(100% - ${EXTENSION_DRAWER_WIDTH.minChat}px));`}
      aria-hidden={!open}
      aria-label={drawer.title}
    >
      <ExtensionDrawerResizeHandle
        title={drawer.title}
        resizing={resizing}
        onPointerDown={handleResizeStart}
      />
      <div
        class={`h-full min-h-0 w-full flex flex-col transition-transform duration-200 ease-out ${open ? "translate-x-0" : "translate-x-full"}`}
      >
        <header class="workspace-pane-header codex-header flex-none bg-surface border-b border-line px-3 md:px-4 pb-2.5 flex items-center gap-2">
          <div class="h-9 w-9 rounded-md bg-tint border border-line grid place-items-center flex-none">
            <span
              aria-hidden="true"
              class="grid h-4 w-4 place-items-center text-accent-blue [&>svg]:h-full [&>svg]:w-full"
              dangerouslySetInnerHTML={{ __html: drawer.icon }}
            />
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2 min-w-0">
              <h2 class="truncate text-[15px] md:text-base font-semibold text-ink-50">{drawer.title}</h2>
              {status && (
                <span class={`h-2 w-2 rounded-full flex-none ${status.active ? "bg-accent-green" : "bg-ink-400"}`} />
              )}
            </div>
            {status && (
              <div class="truncate text-[12px] text-ink-300 font-mono">{status.label}</div>
            )}
          </div>
          <button
            type="button"
            onClick={onClose}
            class="h-9 w-9 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 grid place-items-center"
            title={`Close ${drawer.title}`}
            aria-label={`Close ${drawer.title}`}
            data-workspace-pane-close
          >
            <X class="w-4 h-4" />
          </button>
        </header>

        {/* An <iframe> in the body would swallow the drag's pointer events. */}
        <div
          ref={bodyRef}
          class={`relative flex-1 min-h-0 ${resizing ? "pointer-events-none" : ""}`}
        />
      </div>
    </aside>
  );
}
