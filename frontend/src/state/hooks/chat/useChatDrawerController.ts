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
  const [schedulesOpen, setSchedulesOpen] = useState(false);
  const [terminalOpen, setTerminalOpen] = useState(false);
  const [extensionPaneId, setExtensionPaneId] = useState<string | null>(null);

  useEffect(() => {
    setHistoryOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
    setExtensionPaneId(null);
  }, [chatId]);

  function openBrowser() {
    setHistoryOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
    setExtensionPaneId(null);
    showBrowser();
  }

  function openHistory() {
    hideBrowser();
    setSchedulesOpen(false);
    setTerminalOpen(false);
    setExtensionPaneId(null);
    setHistoryOpen(true);
  }

  function openSchedules() {
    hideBrowser();
    setHistoryOpen(false);
    setTerminalOpen(false);
    setExtensionPaneId(null);
    setSchedulesOpen(true);
  }

  function openTerminal() {
    hideBrowser();
    setHistoryOpen(false);
    setSchedulesOpen(false);
    setExtensionPaneId(null);
    setTerminalOpen(true);
  }

  function openExtensionPane(id: string) {
    hideBrowser();
    setHistoryOpen(false);
    setSchedulesOpen(false);
    setTerminalOpen(false);
    setExtensionPaneId(id);
  }

  return {
    historyOpen,
    schedulesOpen,
    terminalOpen,
    extensionPaneId,
    openBrowser,
    openHistory,
    openSchedules,
    openTerminal,
    openExtensionPane,
    closeHistory: () => setHistoryOpen(false),
    closeSchedules: () => setSchedulesOpen(false),
    closeTerminal: () => setTerminalOpen(false),
    closeExtensionPane: () => setExtensionPaneId(null),
  };
}
