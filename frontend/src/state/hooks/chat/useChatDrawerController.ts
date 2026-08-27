import { useEffect, useState } from "preact/hooks";

export function useChatDrawerController({
  chatId,
  showBrowser,
  hideBrowser,
}: {
  chatId: string;
  showBrowser: () => void;
  hideBrowser: () => void;
}) {
  const [historyOpen, setHistoryOpen] = useState(false);
  const [filesOpen, setFilesOpen] = useState(false);
  const [schedulesOpen, setSchedulesOpen] = useState(false);
  const [terminalOpen, setTerminalOpen] = useState(false);

  useEffect(() => {
    setHistoryOpen(false);
    setFilesOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
  }, [chatId]);

  function openBrowser() {
    setHistoryOpen(false);
    setFilesOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
    showBrowser();
  }

  function openHistory() {
    hideBrowser();
    setFilesOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
    setHistoryOpen(true);
  }

  function openFiles() {
    hideBrowser();
    setHistoryOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
    setFilesOpen(true);
  }

  function openSchedules() {
    hideBrowser();
    setHistoryOpen(false);
    setFilesOpen(false);
    setTerminalOpen(false);
    setSchedulesOpen(true);
  }

  function openTerminal() {
    hideBrowser();
    setHistoryOpen(false);
    setFilesOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(true);
  }

  return {
    historyOpen,
    filesOpen,
    schedulesOpen,
    terminalOpen,
    openBrowser,
    openHistory,
    openFiles,
    openSchedules,
    openTerminal,
    closeHistory: () => setHistoryOpen(false),
    closeFiles: () => setFilesOpen(false),
    closeSchedules: () => setSchedulesOpen(false),
    closeTerminal: () => setTerminalOpen(false),
  };
}
