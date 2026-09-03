import { useStore } from "zustand";
import { useCallback, useEffect, useRef } from "preact/hooks";
import type { ChatStatus, PromptOutcome } from "../../../models/chat";
import { useConfirm } from "../../context/ConfirmContext";
import { chatAttachmentService } from "../../../services/chat/chatAttachmentService.ts";
import { chatComposerSessionStore } from "../../stores/chat/composerSessionStore";
import { promptQueueState } from "./promptQueueState";
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
  refreshMeta,
  attachmentBasePath,
}: {
  chatId: string;
  eventCount: number;
  blockCount: number;
  status: ChatStatus;
  canSendPrompt: boolean;
  sendPrompt: (text: string, clientId?: string) => boolean;
  promptOutcome: PromptOutcome | null;
  rewind: (beforeT: number) => Promise<unknown>;
  refreshMeta: () => Promise<void>;
  attachmentBasePath: string;
}) {
  const confirm = useConfirm();
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
  const upload = useAttachmentUpload(chatId, attachmentBasePath);
  const drag = useDragUpload(upload.doUpload);
  const scroll = useThreadScroll(chatId, `${eventCount}:${blockCount}`);
  const queue = usePromptQueue({
    chatId,
    status,
    canSendPrompt,
    sendPrompt,
    promptOutcome,
    onSent: scroll.unlockAutoScroll,
  });

  useEffect(() => {
    scroll.unlockAutoScroll();
  }, [chatId]);

  async function handleRewind(t: number, promptText: string) {
    if (status === "streaming") {
      alert("Cancel the current run before rewinding this chat.");
      return;
    }
    const confirmed = await confirm({
      title: "Rewind chat",
      description: "This action cannot be undone.",
      message: "Every message from this prompt forward is removed, and the prompt is put back in the composer.",
      confirmLabel: "Rewind",
    });
    if (!confirmed) return;
    try {
      await rewind(t);
      queue.clearQueuedPrompts();
      setText(promptText);
      await refreshMeta();
      scroll.unlockAutoScroll();
      setTimeout(() => {
        scroll.jumpToBottom();
        focusInput();
      }, 0);
    } catch (rewindError) {
      alert("rewind failed: " + (rewindError as Error).message);
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

  function handleAnswerQuestion(answer: string) {
    const sent = sendPrompt(answer);
    if (sent) scroll.unlockAutoScroll();
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
