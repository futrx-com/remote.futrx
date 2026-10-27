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
  const [extensionDrawerId, setExtensionDrawerId] = useState<string | null>(null);

  useEffect(() => {
    setHistoryOpen(false);
    setFilesOpen(false);
    setExtensionDrawerId(null);
  }, [chatId]);

  function openBrowser() {
    setHistoryOpen(false);
    setFilesOpen(false);
    setExtensionDrawerId(null);
    showBrowser();
  }

  function openHistory() {
    hideBrowser();
    setFilesOpen(false);
    setExtensionDrawerId(null);
    setHistoryOpen(true);
  }

  function openFiles() {
    hideBrowser();
    setHistoryOpen(false);
    setExtensionDrawerId(null);
    setFilesOpen(true);
  }

  function openExtensionDrawer(drawerId: string) {
    hideBrowser();
    setHistoryOpen(false);
    setFilesOpen(false);
    setExtensionDrawerId(drawerId);
  }

  return {
    historyOpen,
    filesOpen,
    extensionDrawerId,
    openBrowser,
    openHistory,
    openFiles,
    openExtensionDrawer,
    closeHistory: () => setHistoryOpen(false),
    closeFiles: () => setFilesOpen(false),
    closeExtensionDrawer: () => setExtensionDrawerId(null),
  };
}
