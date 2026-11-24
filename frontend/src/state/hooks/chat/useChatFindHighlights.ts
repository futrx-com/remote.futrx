import type { RefObject } from "preact";
import { useCallback, useEffect, useLayoutEffect, useRef } from "preact/hooks";
import type { ConversationSearchHit } from "../../../api/chat/chatTranscriptApi";
import { CHAT_BLOCK_TIME_ATTRIBUTE, CHAT_FIND_SKIP_SELECTOR } from "../../../config/chat.ts";
import { chatFindHighlightService } from "../../../services/chat/chatFindHighlightService.ts";
import { domTextSearchService } from "../../../services/platform/domTextSearchService.ts";

/** Frames to wait for a jumped-to message to render before giving up. */
const JUMP_RENDER_FRAMES = 60;

const nextFrame = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));

/** The rendered block holding an event at time `t`: the last block that
 *  starts at or before it. */
function blockAt(content: HTMLElement, t: number): HTMLElement | null {
  let found: HTMLElement | null = null;
  for (const element of content.querySelectorAll<HTMLElement>(`[${CHAT_BLOCK_TIME_ATTRIBUTE}]`)) {
    if (Number(element.getAttribute(CHAT_BLOCK_TIME_ATTRIBUTE)) > t) break;
    found = element;
  }
  return found;
}

/** Highlight rendered matches and reveal the selected message after it loads. */
export function useChatFindHighlights({
  contentRef,
  scrollRef,
  open,
  query,
  selectedIndex,
  revision,
  revealHit,
}: {
  contentRef: RefObject<HTMLDivElement>;
  scrollRef: RefObject<HTMLDivElement>;
  open: boolean;
  query: string;
  selectedIndex: number;
  revision: number;
  revealHit: (hit: ConversationSearchHit) => Promise<void>;
}) {
  const currentRangeRef = useRef<Range | null>(null);
  const jumpRef = useRef(0);

  useEffect(() => {
    currentRangeRef.current = null;
    jumpRef.current++;
  }, [query, open]);

  // Keep the browser-owned highlight layers in lockstep with the rendered
  // input. A normal effect runs after paint, which briefly leaves the final
  // one-character match visible when the query changes from "c" to empty.
  useLayoutEffect(() => {
    if (!open || !contentRef.current) {
      chatFindHighlightService.clear();
      return;
    }
    const ranges = domTextSearchService.findRanges(
      contentRef.current,
      query,
      CHAT_FIND_SKIP_SELECTOR
    );
    if (ranges.length === 0) {
      chatFindHighlightService.clear();
      return;
    }
    const current = currentRangeRef.current;
    chatFindHighlightService.show(
      ranges,
      current && current.startContainer.isConnected ? current : null,
    );
  }, [open, query, selectedIndex, revision, contentRef, scrollRef]);

  useEffect(() => () => chatFindHighlightService.clear(), []);

  const jumpTo = useCallback(async (hit: ConversationSearchHit) => {
    const jump = ++jumpRef.current;
    currentRangeRef.current = null;
    await revealHit(hit);
    for (let frame = 0; frame < JUMP_RENDER_FRAMES; frame++) {
      await nextFrame();
      if (jump !== jumpRef.current) return;
      const content = contentRef.current;
      const block = content && blockAt(content, hit.t);
      if (!content || !block) continue;
      const all = domTextSearchService.findRanges(content, query, CHAT_FIND_SKIP_SELECTOR);
      const inBlock = all.find((range) => block.contains(range.startContainer)) ?? null;
      currentRangeRef.current = inBlock;
      chatFindHighlightService.show(all, inBlock);
      if (inBlock) chatFindHighlightService.reveal(scrollRef.current, inBlock);
      // A match in collapsed content, such as a tool output, has no visible
      // text: bring its message into view instead.
      else chatFindHighlightService.revealElement(scrollRef.current, block);
      return;
    }
  }, [contentRef, query, revealHit, scrollRef]);

  return jumpTo;
}
