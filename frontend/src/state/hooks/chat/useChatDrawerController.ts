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
  const [extensionPaneId, setExtensionPaneId] = useState<string | null>(null);

  useEffect(() => {
    setHistoryOpen(false);
    setFilesOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
    setExtensionPaneId(null);
  }, [chatId]);

  function openBrowser() {
    setHistoryOpen(false);
    setFilesOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
    setExtensionPaneId(null);
    showBrowser();
  }

  function openHistory() {
    hideBrowser();
    setFilesOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
    setExtensionPaneId(null);
    setHistoryOpen(true);
  }

  function openFiles() {
    hideBrowser();
    setHistoryOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
    setExtensionPaneId(null);
    setFilesOpen(true);
  }

  function openSchedules() {
    hideBrowser();
    setHistoryOpen(false);
    setFilesOpen(false);
    setTerminalOpen(false);
    setExtensionPaneId(null);
    setSchedulesOpen(true);
  }

  function openTerminal() {
    hideBrowser();
    setHistoryOpen(false);
    setFilesOpen(false);
    setSchedulesOpen(false);
    setExtensionPaneId(null);
    setTerminalOpen(true);
  }

  function openExtensionPane(id: string) {
    hideBrowser();
    setHistoryOpen(false);
    setFilesOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
    setExtensionPaneId(id);
  }

  return {
    historyOpen,
    filesOpen,
    schedulesOpen,
    terminalOpen,
    extensionPaneId,
    openBrowser,
    openHistory,
    openFiles,
    openSchedules,
    openTerminal,
    openExtensionPane,
    closeHistory: () => setHistoryOpen(false),
    closeFiles: () => setFilesOpen(false),
    closeSchedules: () => setSchedulesOpen(false),
    closeTerminal: () => setTerminalOpen(false),
    closeExtensionPane: () => setExtensionPaneId(null),
  };
}
