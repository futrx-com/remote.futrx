import type { ComponentType } from "preact";
import { useEffect, useState } from "preact/hooks";
import type { ChatMeta } from "../../../models/chat";
import { TERMINAL_OVERLAY_LOAD_ERROR_MESSAGE } from "../../../config/terminal";

type TerminalOverlayComponent = ComponentType<{
  chat: ChatMeta;
  open: boolean;
  onClose: () => void;
}>;

export function useTerminalOverlayController(terminalOpen: boolean) {
  const [TerminalOverlay, setTerminalOverlay] = useState<TerminalOverlayComponent | null>(null);
  const [overlayError, setOverlayError] = useState<string | null>(null);
  // Bumped by retryTerminalOverlay so a failed chunk import is attempted
  // again even though terminalOpen did not change.
  const [loadAttempt, setLoadAttempt] = useState(0);

  useEffect(() => {
    if (!terminalOpen || TerminalOverlay) return;
    let cancelled = false;
    setOverlayError(null);
    import("./TerminalOverlay").then(
      (module) => {
        if (!cancelled) setTerminalOverlay(() => module.TerminalOverlay);
      },
      () => {
        // A stale cached bundle after a deploy (or a flaky network) rejects
        // the chunk import. Without surfacing it, the open button appears to
        // do nothing until a page refresh.
        if (!cancelled) setOverlayError(TERMINAL_OVERLAY_LOAD_ERROR_MESSAGE);
      }
    );
    return () => {
      cancelled = true;
    };
  }, [TerminalOverlay, terminalOpen, loadAttempt]);

  function retryTerminalOverlay() {
    if (TerminalOverlay) return;
    setOverlayError(null);
    setLoadAttempt((attempt) => attempt + 1);
  }

  return { TerminalOverlay, overlayError, retryTerminalOverlay };
}
