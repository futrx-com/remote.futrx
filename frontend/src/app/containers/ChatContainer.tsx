import type { ChatMeta } from "../../models/chat";
import type { ProjectMeta } from "../../models/project";
import { useEffect, useRef } from "preact/hooks";
import { BrowserDrawer } from "../../ui/chat/browser/BrowserDrawer";
import { ChatThread } from "../../ui/chat/ChatThread";
import { MediaViewerOverlay } from "../../ui/chat/files/MediaViewerOverlay";
import type { ChatComposerProps } from "../../ui/chat/composer/ChatComposer";
import { WorkspaceActions } from "../../ui/chat/header/WorkspaceActions";
import { HistoryDrawer } from "../../ui/chat/history/HistoryDrawer";
import { FileManagerDrawer } from "../../ui/chat/files/FileManagerDrawer";
import { ScheduleDrawer } from "../../ui/chat/schedules/ScheduleDrawer";
import { chatAttachmentService } from "../../services/chat/chatAttachmentService.ts";
import { useChat } from "../../state/hooks/chat/useChat";
import { useChatBrowserController } from "../../state/hooks/chat/useChatBrowserController";
import { useChatComposerController } from "../../state/hooks/chat/useChatComposerController";
import { useChatDrawerController } from "../../state/hooks/chat/useChatDrawerController";
import { useChatFind } from "../../state/hooks/chat/useChatFind";
import { useChatPreferences } from "../../state/hooks/chat/useChatPreferences";
import { useChatReadMarker } from "../../state/hooks/chat/useChatReadMarker";
import { useDismissShortcut } from "../../state/hooks/shared/useDismissShortcut.ts";
import { useSlashCommandMenu } from "../../state/hooks/chat/useSlashCommandMenu";
import { useTerminalOverlayController } from "../../ui/chat/terminal/useTerminalOverlayController";
import { useWorkspaceGitRepos } from "../../state/hooks/chat/useWorkspaceGitRepos";

export function ChatContainer({
  chat,
  projects,
  onHamburger,
}: {
  chat: ChatMeta;
  projects: ProjectMeta[];
  onHamburger: () => void;
}) {
  const {
    meta,
    blocks,
    eventCount,
    hasOlder,
    loadingOlder,
    status,
    error,
    canSendPrompt,
    sendPrompt,
    promptOutcome,
    cancel,
    respondInteraction,
    rewind,
    loadOlder,
    refreshMeta,
  } = useChat(chat.id);
  const preferences = useChatPreferences({ chat, loadedMeta: meta, refreshMeta });
  const { displayMeta, displayMode, selectedSkills } = preferences;
  const attachmentBasePath = chatAttachmentService.basePath(displayMeta, projects);
  const project = projects.find((candidate) => candidate.id === displayMeta.projectId);
  const composer = useChatComposerController({
    chatId: chat.id,
    eventCount,
    blockCount: blocks.length,
    status,
    canSendPrompt,
    sendPrompt,
    promptOutcome,
    rewind,
    refreshMeta,
    attachmentBasePath,
  });
  const slashCommandMenu = useSlashCommandMenu({
    provider: displayMeta.provider || "codex",
    projectId: displayMeta.projectId,
    text: composer.text,
    onSelectSkill: preferences.selectSkill,
    onTextChange: composer.setText,
    focusTextarea: () => composer.textareaRef.current?.focus(),
  });
  const browser = useChatBrowserController({
    chat: displayMeta,
    projects,
    blocks,
    text: composer.text,
    setText: composer.setText,
    textareaRef: composer.textareaRef,
  });
  const drawers = useChatDrawerController({
    chatId: chat.id,
    showBrowser: browser.openBrowserDrawer,
    hideBrowser: browser.closeBrowserDrawer,
  });
  const terminal = useTerminalOverlayController(drawers.terminalOpen);

  // `eventCount` stands in for "the thread changed": find re-reads the rendered
  // messages on it, so a match list cannot go stale against a streaming reply.
  const find = useChatFind({
    scrollRef: composer.scroll.scrollRef,
    contentRef: composer.scroll.contentRef,
    revision: eventCount,
  });

  useChatReadMarker({ chatId: chat.id, eventCount, status });
  // Escape cancels the reply being streamed, and is the weakest claim on the
  // key in a chat: it falls behind find-in-chat, a menu, and every modal, so
  // Escape only reaches the run when nothing is open over it.
  useDismissShortcut(cancel, { enabled: status === "streaming", fallback: true });
  const { hasRepos } = useWorkspaceGitRepos({ chatId: chat.id, status });
  const workspaceActions = {
    cwd: displayMeta.cwd || "~",
    onToggleTerminal: drawers.terminalOpen ? drawers.closeTerminal : drawers.openTerminal,
    onToggleBrowser: browser.browserOpen ? browser.closeBrowserDrawer : drawers.openBrowser,
    onToggleHistory: drawers.historyOpen ? drawers.closeHistory : drawers.openHistory,
    onToggleFiles: drawers.filesOpen ? drawers.closeFiles : drawers.openFiles,
    onToggleSchedules: drawers.schedulesOpen ? drawers.closeSchedules : drawers.openSchedules,
    terminalOpen: drawers.terminalOpen,
    browserOpen: browser.browserOpen,
    historyOpen: drawers.historyOpen,
    filesOpen: drawers.filesOpen,
    schedulesOpen: drawers.schedulesOpen,
    showHistory: hasRepos,
    showSchedules: !!displayMeta.projectId,
  };
  const activePane = drawers.historyOpen
    ? "history"
    : drawers.filesOpen
      ? "files"
      : drawers.schedulesOpen
        ? "schedules"
        : drawers.terminalOpen
          ? "terminal"
          : browser.browserOpen
            ? "browser"
            : null;
  const previousMobilePane = useRef<typeof activePane>(null);

  useEffect(() => {
    if (!window.matchMedia("(max-width: 767px)").matches) return;

    if (activePane) {
      previousMobilePane.current = activePane;
      requestAnimationFrame(() => {
        document
          .getElementById(`workspace-${activePane}-pane`)
          ?.querySelector<HTMLElement>("[data-workspace-pane-close]")
          ?.focus();
      });
      return;
    }

    const closedPane = previousMobilePane.current;
    if (!closedPane) return;
    previousMobilePane.current = null;
    requestAnimationFrame(() => {
      const triggers = document.querySelectorAll<HTMLElement>(
        `[data-workspace-action="${closedPane}"]`
      );
      Array.from(triggers).find((trigger) => trigger.offsetParent !== null)?.focus();
    });
  }, [activePane]);

  const composerView: ChatComposerProps = {
    projectId: displayMeta.projectId,
    streaming: status === "streaming",
    canSendPrompt,
    preferences: {
      provider: displayMeta.provider || "codex",
      model: displayMeta.model || "",
      mode: displayMode,
      reasoningEffort: displayMeta.reasoningEffort || "",
      serviceTier: displayMeta.serviceTier || "",
      approvalPolicy: displayMeta.approvalPolicy,
      sandboxPolicy: displayMeta.sandboxPolicy,
    },
    preferenceActions: {
      changeAgent: preferences.changeAgent,
      changeMode: preferences.changeMode,
      changeReasoningEffort: preferences.changeReasoningEffort,
      changeServiceTier: preferences.changeServiceTier,
      changeApprovalPolicy: preferences.changeApprovalPolicy,
      changeSandboxPolicy: preferences.changeSandboxPolicy,
    },
    queuedPrompts: composer.queue.queuedPrompts,
    selectedSkills,
    attachments: composer.upload.attachments,
    uploading: composer.upload.uploading,
    dragging: composer.drag.dragging,
    text: composer.text,
    textareaRef: composer.textareaRef,
    fileInputRef: composer.fileInputRef,
    onTextChange: composer.setText,
    onFilesSelected: composer.upload.doUpload,
    onPaste: composer.handlePaste,
    onSend: composer.handleSend,
    onCancel: cancel,
    onRemoveQueued: composer.queue.removeQueuedPrompt,
    onRemoveAttachment: composer.upload.removeAttachment,
    onSelectSkill: preferences.selectSkill,
    onRemoveSelectedSkill: preferences.removeSelectedSkill,
    slashCommandMenu: {
      open: slashCommandMenu.open,
      loading: slashCommandMenu.loading,
      error: slashCommandMenu.error,
      query: slashCommandMenu.query,
      items: slashCommandMenu.items,
      highlight: slashCommandMenu.highlight,
      onHighlight: slashCommandMenu.setHighlight,
      onChoose: slashCommandMenu.choose,
      onKeyDown: slashCommandMenu.onKeyDown,
    },
  };

  return (
    <div class="relative flex-1 h-full min-h-0 overflow-hidden">
      <div class="flex h-full min-h-0 w-full overflow-hidden">
        <div class={`min-w-0 flex-1 h-full ${activePane ? "hidden md:block" : ""}`}>
          <ChatThread
            find={find}
            chat={displayMeta}
            blocks={blocks}
            hasOlder={hasOlder}
            loadingOlder={loadingOlder}
            status={status}
            error={error}
            composer={composerView}
            showJump={composer.scroll.showJump}
            scrollRef={composer.scroll.scrollRef}
            contentRef={composer.scroll.contentRef}
            bottomRef={composer.scroll.bottomRef}
            onHamburger={onHamburger}
            onScroll={composer.scroll.onScroll}
            onJumpToBottom={composer.scroll.jumpToBottom}
            onAnswerQuestion={composer.handleAnswerQuestion}
            onRespondInteraction={respondInteraction}
            onLoadOlder={loadOlder}
            onRewind={composer.handleRewind}
            projectName={project?.name}
            actions={<WorkspaceActions {...workspaceActions} orientation="horizontal" />}
          />
        </div>
        <HistoryDrawer
          chatId={chat.id}
          open={drawers.historyOpen}
          onClose={drawers.closeHistory}
        />
        <FileManagerDrawer
          chatId={chat.id}
          open={drawers.filesOpen}
          onClose={drawers.closeFiles}
        />
        <ScheduleDrawer
          chatId={chat.id}
          open={drawers.schedulesOpen}
          onClose={drawers.closeSchedules}
        />
        <BrowserDrawer
          open={browser.browserOpen}
          projectId={browser.browserProject?.id || ""}
          projectName={browser.browserProject?.name || ""}
          projectSlug={browser.browserProject?.slug || ""}
          apps={browser.containerApps}
          appsLoading={browser.appsLoading}
          selectedPort={browser.selectedAppPort}
          onSelectPort={browser.setSelectedAppPort}
          onRefreshApps={() => void browser.loadContainerApps()}
          onCaptureElement={browser.insertBrowserElementContext}
          onClose={browser.closeBrowserDrawer}
        />
        {terminal.TerminalOverlay && (
          <terminal.TerminalOverlay
            chat={displayMeta}
            open={drawers.terminalOpen}
            onClose={drawers.closeTerminal}
          />
        )}
      </div>
      <MediaViewerOverlay />
    </div>
  );
}
