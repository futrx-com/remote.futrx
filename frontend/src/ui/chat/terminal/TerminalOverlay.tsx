import { useEffect, useRef, useState } from "preact/hooks";
import "@xterm/xterm/css/xterm.css";
import type { ChatMeta } from "../../../models/chat";
import { useTerminalSession } from "../../../state/hooks/chat/useTerminalSession";
import { Terminal as TerminalIcon, X } from "../../primitives/icons";
import { TerminalResizeHandle } from "./TerminalResizeHandle";

const terminalWidthKey = "remote.futrx.terminalDrawerWidth";
const defaultTerminalWidth = 560;
const minTerminalWidth = 420;
const maxTerminalWidth = 1100;
const minChatWidth = 360;

function clampWidth(width: number, maxWidth = maxTerminalWidth): number {
  return Math.min(Math.max(width, minTerminalWidth), Math.max(minTerminalWidth, maxWidth));
}

function readTerminalWidth(): number {
  if (typeof window === "undefined") return defaultTerminalWidth;
  const stored = Number(window.localStorage.getItem(terminalWidthKey));
  return Number.isFinite(stored) && stored > 0 ? clampWidth(stored) : defaultTerminalWidth;
}

export function TerminalOverlay({
  chat,
  open,
  onClose,
}: {
  chat: ChatMeta;
  open: boolean;
  onClose: () => void;
}) {
  // Keep the session enabled for this chat once it has been opened, so the
  // running shell — including anything typed but not yet submitted — survives
  // closing and reopening the pane. It is only torn down when the chat changes.
  const [openedChatId, setOpenedChatId] = useState<string | null>(() => (open ? chat.id : null));
  const terminal = useTerminalSession({
    chatId: chat.id,
    enabled: openedChatId === chat.id,
    title: chat.title,
  });
  const [terminalWidth, setTerminalWidth] = useState(readTerminalWidth);
  const [resizing, setResizing] = useState(false);
  const asideRef = useRef<HTMLElement>(null);

  useEffect(() => {
    if (open) {
      setOpenedChatId(chat.id);
      return;
    }
    setOpenedChatId((current) => (current === chat.id ? current : null));
  }, [chat.id, open]);

  useEffect(() => {
    if (!open) return;
    const frame = requestAnimationFrame(terminal.focus);
    return () => cancelAnimationFrame(frame);
  }, [open, terminal.focus]);

  useEffect(() => {
    window.localStorage.setItem(terminalWidthKey, String(terminalWidth));
  }, [terminalWidth]);

  // Keep the pane within the container as the viewport changes, always leaving
  // room for the chat beside it.
  useEffect(() => {
    if (!open) return;
    function clampToContainer() {
      const bounds = asideRef.current?.parentElement?.getBoundingClientRect();
      if (!bounds) return;
      const availableWidth = Math.min(maxTerminalWidth, Math.max(minTerminalWidth, bounds.width - minChatWidth));
      setTerminalWidth((width) => clampWidth(width, availableWidth));
    }
    clampToContainer();
    window.addEventListener("resize", clampToContainer);
    return () => window.removeEventListener("resize", clampToContainer);
  }, [open]);

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
      const availableWidth = Math.min(maxTerminalWidth, Math.max(minTerminalWidth, bounds.width - minChatWidth));
      // Dragging the left edge: width grows as the pointer moves left.
      const next = bounds.right - moveEvent.clientX;
      setTerminalWidth(clampWidth(next, availableWidth));
    }

    window.addEventListener("pointermove", resize, { passive: false });
    window.addEventListener("pointerup", finishResize);
    window.addEventListener("pointercancel", finishResize);
  }

  const workspacePath = chat.cwd || "/workspace";
  const statusLabel =
    terminal.status === "connected" ? "Connected" :
    terminal.status === "connecting" ? "Connecting" :
    terminal.status === "error" ? "Error" :
    "Closed";

  return (
    <aside
      ref={asideRef}
      id="workspace-terminal-pane"
      class={`workspace-pane workspace-terminal-pane relative z-20 h-full flex-none overflow-hidden bg-surface border-l border-line
              ${resizing ? "transition-none" : "transition-[width,opacity] duration-200 ease-out"}
              ${open ? "opacity-100 shadow-2xl" : "opacity-0 border-l-0 shadow-none pointer-events-none"}`}
      style={`--workspace-terminal-width: ${terminalWidth}px; --workspace-terminal-max-width: max(${minTerminalWidth}px, calc(100% - ${minChatWidth}px));`}
      aria-hidden={!open}
      aria-label="Terminal"
    >
      <TerminalResizeHandle resizing={resizing} onPointerDown={handleResizeStart} />
      <div
        class={`h-full min-h-0 w-full flex flex-col transition-transform duration-200 ease-out ${open ? "translate-x-0" : "translate-x-full"}`}
      >
        <header class="workspace-pane-header codex-header flex-none bg-surface border-b border-line px-3 md:px-4 pb-2.5 flex items-center gap-2">
          <div class="h-9 w-9 rounded-md bg-tint border border-line grid place-items-center flex-none">
            <TerminalIcon class="w-4 h-4 text-accent-blue" />
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2 min-w-0">
              <h2 class="truncate text-[15px] md:text-base font-semibold text-ink-50">Terminal</h2>
              <span class={`h-2 w-2 rounded-full flex-none ${terminal.status === "connected" ? "bg-accent-green" : "bg-ink-400"}`} />
            </div>
            <div class="truncate text-[12px] text-ink-300 font-mono">
              {statusLabel} - {workspacePath}
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            class="h-9 w-9 rounded-md bg-tint hover:bg-tint-strong border border-line text-ink-200 grid place-items-center"
            title="Close terminal"
            aria-label="Close terminal"
            data-workspace-pane-close
          >
            <X class="w-4 h-4" />
          </button>
        </header>

        {terminal.error && (
          <div class="flex-none mx-3 md:mx-4 mt-3 rounded-md border border-accent-red/30 bg-accent-red/10 px-3 py-2 text-sm text-accent-red">
            {terminal.error}
          </div>
        )}

        <div class="flex-1 min-h-0 p-2 md:p-3">
          <div
            ref={terminal.hostRef}
            class="h-full w-full overflow-hidden rounded-md border border-line bg-inset p-2"
          />
        </div>
      </div>
    </aside>
  );
}
