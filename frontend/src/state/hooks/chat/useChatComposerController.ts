import { useStore } from "zustand";
import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import type { ChatStatus, PromptOutcome } from "../../../models/chat";
import { useConfirm } from "../../context/ConfirmContext";
import { chatAttachmentService } from "../../../services/chat/chatAttachmentService.ts";
import { chatComposerSessionStore } from "../../stores/chat/composerSessionStore";
import { promptQueueState } from "./promptQueueState";
import { runChatRewind } from "./chatRewindFlow";
import { useAttachmentUpload } from "./useAttachmentUpload";
import { useAutosizeTextarea } from "./useAutosizeTextarea";
import { useDragUpload } from "./useDragUpload";
import { usePromptQueue } from "./usePromptQueue";
import { useThreadScroll } from "./useThreadScroll";

export function useChatComposerController({
  chatId,
  eventCount,
  blockCount,
  status,
  canSendPrompt,
  sendPrompt,
  promptOutcome,
  rewind,
  stopActiveRun,
  refreshMeta,
  attachmentBasePath,
  projectId,
}: {
  chatId: string;
  eventCount: number;
  blockCount: number;
  status: ChatStatus;
  canSendPrompt: boolean;
  sendPrompt: (text: string, clientId?: string) => boolean;
  promptOutcome: PromptOutcome | null;
  rewind: (rewindBeforeTimestamp: number) => Promise<unknown>;
  stopActiveRun: () => Promise<void>;
  refreshMeta: (throwOnError?: boolean) => Promise<void>;
  attachmentBasePath: string;
  /**
   * Carried through to the upload hook so an extension can tell which
   * project's plugin an attachment belongs to.
   */
  projectId?: string;
}) {
  const confirm = useConfirm();
  const [rewindPending, setRewindPending] = useState(false);
  const rewindPendingRef = useRef(false);
  // ChatContainer remounts on chat switch (it is keyed by chatId), so selecting
  // the active draft from the session store is what makes a half-typed message
  // survive leaving and returning to a chat.
  const text = useStore(
    chatComposerSessionStore,
    (state) => state.drafts.get(chatId) ?? "",
  );
  const setDraft = useStore(chatComposerSessionStore, (state) => state.setDraft);
  const setText = useCallback(
    (value: string | ((prev: string) => string)) => {
      const previous = chatComposerSessionStore.getState().drafts.get(chatId) ?? "";
      const next = typeof value === "function" ? value(previous) : value;
      setDraft(chatId, next);
    },
    [chatId, setDraft],
  );
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { textareaRef, focusInput } = useAutosizeTextarea(text);
  const upload = useAttachmentUpload(chatId, attachmentBasePath, projectId);
  const drag = useDragUpload(upload.doUpload);
  const scroll = useThreadScroll(chatId, `${eventCount}:${blockCount}`);
  const queue = usePromptQueue({
    chatId,
    status,
    canSendPrompt,
    suspended: rewindPending,
    sendPrompt,
    promptOutcome,
    onSent: scroll.unlockAutoScroll,
  });

  useEffect(() => {
    scroll.unlockAutoScroll();
  }, [chatId]);

  async function handleRewind(rewindBeforeTimestamp: number, promptText: string) {
    // The server may become idle between cancellation and rewind. Hold queued
    // prompts during the whole dialog so none start a new run in that gap.
    if (rewindPendingRef.current) return;
    rewindPendingRef.current = true;
    setRewindPending(true);
    try {
      await runChatRewind({
        status,
        rewindBeforeTimestamp,
        promptText,
        confirm,
        stopAgent: stopActiveRun,
        rewind,
        clearQueuedPrompts: queue.clearQueuedPrompts,
        restorePrompt: setText,
        refreshMeta,
        onComplete: () => {
          scroll.unlockAutoScroll();
          setTimeout(() => {
            scroll.jumpToBottom();
            focusInput();
          }, 0);
        },
      });
    } finally {
      rewindPendingRef.current = false;
      setRewindPending(false);
    }
  }

  function handlePaste(event: ClipboardEvent) {
    const items = event.clipboardData?.items;
    if (!items) return;
    const files: File[] = [];
    for (let i = 0; i < items.length; i++) {
      const item = items[i];
      if (item.kind === "file") {
        const file = item.getAsFile();
        if (file) files.push(file);
      }
    }
    if (files.length) {
      event.preventDefault();
      upload.doUpload(files);
    }
  }

  function handleSend() {
    if (upload.uploading || (!promptQueueState.allowsQueue(status) && !canSendPrompt)) return;
    const userText = text.trim();
    const paths = upload.attachments
      .filter((attachment) => attachment.serverPath)
      .map((attachment) => attachment.serverPath);
    if (!userText && paths.length === 0) return;
    const finalText = paths.length
      ? chatAttachmentService.promptWithAttachments(userText, paths)
      : userText;

    if (status === "streaming") {
      queue.queuePrompt(finalText);
    } else {
      const sent = sendPrompt(finalText);
      if (!sent) return;
    }

    setText("");
    upload.clearAttachments();
    scroll.unlockAutoScroll();
    setTimeout(focusInput, 0);
  }

  function handleAnswerQuestion(answer: string): boolean {
    const sent = sendPrompt(answer);
    if (sent) scroll.unlockAutoScroll();
    return sent;
  }

  return {
    text,
    setText,
    textareaRef,
    fileInputRef,
    upload,
    drag,
    scroll,
    queue,
    handlePaste,
    handleSend,
    handleAnswerQuestion,
    handleRewind,
  };
}
