import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { FitAddon } from "@xterm/addon-fit";
import { Terminal as XTerm } from "@xterm/xterm";
import { terminalApi } from "../../../api/terminalApi";
import type { TerminalConnection, TerminalStatus } from "../../../types/terminal";
import {
  TERMINAL_CONNECTION_ERROR_MESSAGE,
  TERMINAL_DEFAULT_TITLE,
  TERMINAL_INITIAL_FIT_DELAY_MS,
  TERMINAL_OPTIONS,
  TERMINAL_STATUS,
  TERMINAL_THEME,
} from "../../../config/terminal";

function createTerminal(): XTerm {
  return new XTerm({
    ...TERMINAL_OPTIONS,
    theme: TERMINAL_THEME,
  });
}

export function useTerminalSession({
  chatId,
  enabled,
  title,
}: {
  chatId: string;
  enabled: boolean;
  title?: string;
}) {
  const hostRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<XTerm | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const connectionRef = useRef<TerminalConnection | null>(null);
  const titleRef = useRef(title);
  const [status, setStatus] = useState<TerminalStatus>(TERMINAL_STATUS.closed);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    titleRef.current = title;
  }, [title]);

  const fitAndResize = useCallback(() => {
    const terminal = terminalRef.current;
    const fit = fitRef.current;
    const connection = connectionRef.current;
    if (!terminal || !fit) return;
    // When the docked pane is collapsed its host measures 0×0. Fitting to that
    // would resize the PTY to zero columns and mangle the shell's current line,
    // so skip until the pane is visible again.
    const host = hostRef.current;
    if (host && (host.clientWidth === 0 || host.clientHeight === 0)) return;
    try {
      fit.fit();
      if (connection?.isOpen) {
        connection.resize(terminal.cols, terminal.rows);
      }
    } catch {}
  }, []);

  const focus = useCallback(() => {
    terminalRef.current?.focus();
    fitAndResize();
  }, [fitAndResize]);

  useEffect(() => {
    if (!enabled) {
      setStatus(TERMINAL_STATUS.closed);
      setError(null);
      return;
    }
    if (!hostRef.current) return;

    let disposed = false;
    const terminal = createTerminal();
    const fit = new FitAddon();
    terminal.loadAddon(fit);
    terminal.open(hostRef.current);
    terminalRef.current = terminal;
    fitRef.current = fit;

    const connection = terminalApi.connect(chatId, {
      onOpen() {
        if (disposed) return;
        setStatus(TERMINAL_STATUS.connected);
        terminal.writeln(
          `Connected to ${titleRef.current || TERMINAL_DEFAULT_TITLE} terminal`
        );
        terminal.writeln("");
        fitAndResize();
      },
      onOutput(data) {
        if (disposed) return;
        terminal.write(data);
      },
      onError() {
        if (disposed) return;
        setError(TERMINAL_CONNECTION_ERROR_MESSAGE);
        setStatus(TERMINAL_STATUS.error);
      },
      onClose() {
        if (disposed) return;
        setStatus((current) =>
          current === TERMINAL_STATUS.error ? current : TERMINAL_STATUS.closed
        );
      },
    });
    connectionRef.current = connection;
    setStatus(TERMINAL_STATUS.connecting);
    setError(null);

    const inputSub = terminal.onData((data) => {
      if (connection.isOpen) {
        connection.sendInput(data);
      }
    });

    const resizeObserver = new ResizeObserver(() => {
      if (connection.isOpen) fitAndResize();
    });
    resizeObserver.observe(hostRef.current);
    window.setTimeout(fitAndResize, TERMINAL_INITIAL_FIT_DELAY_MS);

    return () => {
      disposed = true;
      resizeObserver.disconnect();
      inputSub.dispose();
      connection.close();
      terminal.dispose();
      connectionRef.current = null;
      terminalRef.current = null;
      fitRef.current = null;
    };
  }, [chatId, enabled, fitAndResize]);

  return {
    hostRef,
    status,
    error,
    focus,
  };
}
