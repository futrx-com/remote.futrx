import type { RefObject } from "preact";
import { useEffect, useMemo, useState } from "preact/hooks";
import type { ChatStatus } from "../../../models/chat";
import type { ChatMessageBlock } from "../../../models/chatMessage";
import { MessageBlock } from "./MessageBlock";
import { MessageSkeleton } from "./MessageSkeleton";
import { ThreadEmptyState } from "./ThreadEmptyState";

const INITIAL_VISIBLE_BLOCKS = 80;
const LOAD_MORE_BLOCKS = 80;

export function MessageList({
  status,
  blocks,
  hasOlder,
  loadingOlder,
  error,
  chatId,
  cwd,
  scrollRef,
  contentRef,
  bottomRef,
  onScroll,
  onAnswerQuestion,
  onLoadOlder,
  onRewind,
}: {
  status: ChatStatus;
  blocks: ChatMessageBlock[];
  hasOlder: boolean;
  loadingOlder: boolean;
  error: string | null;
  chatId: string;
  cwd?: string;
  scrollRef: RefObject<HTMLDivElement>;
  contentRef: RefObject<HTMLDivElement>;
  bottomRef: RefObject<HTMLDivElement>;
  onScroll: () => void;
  onAnswerQuestion: (text: string) => void;
  onLoadOlder: () => Promise<void>;
  onRewind: (t: number, text: string) => void;
}) {
  const [visibleBlockCount, setVisibleBlockCount] = useState(INITIAL_VISIBLE_BLOCKS);
  const firstVisibleIndex = Math.max(0, blocks.length - visibleBlockCount);
  const hiddenCount = firstVisibleIndex;
  const visibleBlocks = useMemo(
    () => blocks.slice(firstVisibleIndex),
    [blocks, firstVisibleIndex]
  );

  useEffect(() => {
    setVisibleBlockCount(INITIAL_VISIBLE_BLOCKS);
  }, [chatId]);

  async function showOlder() {
    if (hiddenCount > 0) {
      setVisibleBlockCount((count) => count + LOAD_MORE_BLOCKS);
      return;
    }
    const element = scrollRef.current;
    const beforeHeight = element?.scrollHeight ?? 0;
    const beforeTop = element?.scrollTop ?? 0;
    await onLoadOlder();
    setVisibleBlockCount((count) => count + LOAD_MORE_BLOCKS);
    requestAnimationFrame(() => {
      const next = scrollRef.current;
      if (!next) return;
      next.scrollTop = beforeTop + next.scrollHeight - beforeHeight;
    });
  }

  return (
    <div
      ref={scrollRef}
      onScroll={onScroll}
      class="codex-message-scroll h-full overflow-y-auto touch-scroll scrollbar-thin px-3 pb-6 pt-4 sm:px-5 md:px-8 md:pt-7"
    >
      {/* A measured column: long assistant prose stays readable on wide panes. */}
      <div ref={contentRef} class="mx-auto w-full max-w-[54rem] space-y-5 md:space-y-6">
        {status === "loading" && <MessageSkeleton />}

        {status !== "loading" && blocks.length === 0 && <ThreadEmptyState cwd={cwd} />}

        {(hiddenCount > 0 || hasOlder) && (
          <div class="flex justify-center">
            <button
              type="button"
              onClick={showOlder}
              disabled={loadingOlder}
              class="h-8 rounded-control px-3 text-[12px] text-ink-400 transition-colors hover:bg-tint-strong hover:text-ink-100"
            >
              {hiddenCount > 0
                ? `Show ${Math.min(hiddenCount, LOAD_MORE_BLOCKS)} older message${Math.min(hiddenCount, LOAD_MORE_BLOCKS) === 1 ? "" : "s"}`
                : loadingOlder
                  ? "Loading older messages"
                  : "Load older messages"}
            </button>
          </div>
        )}

        {visibleBlocks.map((block, index) => {
          const blockIndex = firstVisibleIndex + index;
          return (
            <MessageBlock
              key={`${block.type}-${block.t}-${blockIndex}`}
              block={block}
              streaming={status === "streaming" && blockIndex === blocks.length - 1}
              chatId={chatId}
              cwd={cwd}
              onAnswerQuestion={onAnswerQuestion}
              onRewind={onRewind}
            />
          );
        })}

        {error && (
          <div class="rounded-card border border-accent-red/25 bg-accent-red/[0.08] p-3 text-[13px] text-accent-red [overflow-wrap:anywhere]">
            {error}
          </div>
        )}

        <div ref={bottomRef} class="h-px" aria-hidden="true" />
      </div>
    </div>
  );
}
