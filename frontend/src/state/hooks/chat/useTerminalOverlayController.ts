import type { ComponentType } from "preact";
import { useEffect, useState } from "preact/hooks";
import type { ChatMeta } from "../../../models/chat";

type TerminalOverlayComponent = ComponentType<{
  chat: ChatMeta;
  open: boolean;
  onClose: () => void;
}>;

export function useTerminalOverlayController(terminalOpen: boolean) {
  const [TerminalOverlay, setTerminalOverlay] = useState<TerminalOverlayComponent | null>(null);

  useEffect(() => {
    if (!terminalOpen || TerminalOverlay) return;
    let cancelled = false;
    import("../../../ui/chat/terminal/TerminalOverlay").then((module) => {
      if (!cancelled) setTerminalOverlay(() => module.TerminalOverlay);
    });
    return () => {
      cancelled = true;
    };
  }, [TerminalOverlay, terminalOpen]);

  return { TerminalOverlay };
}
