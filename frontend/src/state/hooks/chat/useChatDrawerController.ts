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
  const [extensionDrawerId, setExtensionDrawerId] = useState<string | null>(null);

  useEffect(() => {
    setHistoryOpen(false);
    setExtensionDrawerId(null);
  }, [chatId]);

  function openBrowser() {
    setHistoryOpen(false);
    setExtensionDrawerId(null);
    showBrowser();
  }

  function openHistory() {
    hideBrowser();
    setExtensionDrawerId(null);
    setHistoryOpen(true);
  }

  function openExtensionDrawer(drawerId: string) {
    hideBrowser();
    setHistoryOpen(false);
    setExtensionDrawerId(drawerId);
  }

  return {
    historyOpen,
    extensionDrawerId,
    openBrowser,
    openHistory,
    openExtensionDrawer,
    closeHistory: () => setHistoryOpen(false),
    closeExtensionDrawer: () => setExtensionDrawerId(null),
  };
}
