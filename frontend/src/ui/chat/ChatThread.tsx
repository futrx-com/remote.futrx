import type { ComponentChildren, RefObject } from "preact";
import type { ChatMeta, ChatStatus } from "../../models/chat";
import type { ChatMessageBlock } from "../../models/chatMessage";
import type { ChatFind } from "../../state/hooks/chat/useChatFind";
import { ChatComposer, type ChatComposerProps } from "./composer/ChatComposer";
import { ChatFindBar } from "./find/ChatFindBar";
import { JumpToLatestButton } from "./messages/JumpToLatestButton";
import { MessageList } from "./messages/MessageList";
import { ThreadHeader } from "./header/ThreadHeader";
import type { ChatInteractionResponder } from "../../types/chatApi";

export function ChatThread({
  chat,
  find,
  blocks,
  hasOlder,
  loadingOlder,
  status,
  error,
  composer,
  showJump,
  scrollRef,
  contentRef,
  bottomRef,
  onHamburger,
  onScroll,
  onJumpToBottom,
  onAnswerQuestion,
  onRespondInteraction,
  onLoadOlder,
  onRewind,
  actions,
  projectName,
}: {
  chat: ChatMeta;
  find: ChatFind;
  blocks: ChatMessageBlock[];
  hasOlder: boolean;
  loadingOlder: boolean;
  status: ChatStatus;
  error: string | null;
  composer: ChatComposerProps;
  showJump: boolean;
  scrollRef: RefObject<HTMLDivElement>;
  contentRef: RefObject<HTMLDivElement>;
  bottomRef: RefObject<HTMLDivElement>;
  onHamburger: () => void;
  onScroll: () => void;
  onJumpToBottom: () => void;
  onAnswerQuestion: (text: string) => void;
  onRespondInteraction?: ChatInteractionResponder;
  onLoadOlder: () => Promise<void>;
  onRewind: (t: number, text: string) => void;
  /** Workspace controls. Rendered in the header on desktop and in the toolbar
   *  strip below it on mobile — only ever one of the two is visible. */
  actions: ComponentChildren;
  projectName?: string;
}) {
  return (
    <div class="codex-thread flex-1 h-full flex min-h-0 overflow-hidden bg-canvas">
      <div class="flex min-w-0 flex-1 flex-col">
        <ThreadHeader
          chat={chat}
          streaming={composer.streaming}
          projectName={projectName}
          actions={actions}
          onHamburger={onHamburger}
        />
        <div class="workspace-action-toolbar relative z-30 flex flex-none justify-end border-b border-line px-2.5 py-1.5 md:hidden">
          {actions}
        </div>

        <div class="relative flex-1 min-h-0">
          <MessageList
            status={status}
            blocks={blocks}
            hasOlder={hasOlder}
            loadingOlder={loadingOlder}
            error={error}
            chatId={chat.id}
            cwd={chat.cwd}
            scrollRef={scrollRef}
            contentRef={contentRef}
            bottomRef={bottomRef}
            onScroll={onScroll}
            onAnswerQuestion={onAnswerQuestion}
            onRespondInteraction={onRespondInteraction}
            onLoadOlder={onLoadOlder}
            onRewind={onRewind}
          />
          <ChatFindBar find={find} hasUnloadedMessages={hasOlder} />
          {showJump && <JumpToLatestButton onClick={onJumpToBottom} />}
        </div>

        <ChatComposer {...composer} />
      </div>
    </div>
  );
}
