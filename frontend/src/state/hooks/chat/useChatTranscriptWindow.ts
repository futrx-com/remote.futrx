import { useCallback, useEffect, useRef, useState, type Dispatch, type MutableRef, type StateUpdater } from "preact/hooks";
import { chatApi } from "../../../api/chatApi";
import { CHAT_JUMP_CONTEXT_TURNS, CHAT_TRANSCRIPT_TURN_PAGE_LIMIT } from "../../../config/api.ts";
import type { ChatEvent, ChatRenderState } from "../../../models/chat";
import { chatEventStateProjector } from "./chatEventStateProjector";

/** Page the displayed transcript and buffer live events while viewing an older window. */
export function useChatTranscriptWindow({
  chatId,
  renderState,
  setRenderState,
  historyGenerationRef,
  historyNeedsRefreshRef,
}: {
  chatId: string;
  renderState: ChatRenderState;
  setRenderState: Dispatch<StateUpdater<ChatRenderState>>;
  historyGenerationRef: MutableRef<number>;
  historyNeedsRefreshRef: MutableRef<boolean>;
}) {
  const [loadingOlder, setLoadingOlder] = useState(false);
  const [loadingNewer, setLoadingNewer] = useState(false);
  const [hasNewer, setHasNewer] = useState(false);
  const renderStateRef = useRef(renderState);
  renderStateRef.current = renderState;

  // A non-null buffer indicates a gap between this window and the live tail.
  const bufferedEventsRef = useRef<ChatEvent[] | null>(null);
  const isDetached = useCallback(() => bufferedEventsRef.current !== null, []);
  const bufferLiveEvents = useCallback((events: ChatEvent[]) => {
    const buffer = bufferedEventsRef.current;
    if (!buffer) return false;
    buffer.push(...events);
    return true;
  }, []);

  const resetWindow = useCallback(() => {
    bufferedEventsRef.current = null;
    setHasNewer(false);
  }, []);
  const resetOlderLoad = useCallback(() => setLoadingOlder(false), []);

  useEffect(() => {
    resetOlderLoad();
    resetWindow();
    setLoadingNewer(false);
  }, [chatId, resetOlderLoad, resetWindow]);

  const loadOlder = useCallback(async () => {
    if (loadingOlder || historyNeedsRefreshRef.current || !renderState.hasOlder || !renderState.nextBefore) return;
    const generation = historyGenerationRef.current;
    setLoadingOlder(true);
    try {
      const page = await chatApi.fetchTranscript(chatId, {
        limit: CHAT_TRANSCRIPT_TURN_PAGE_LIMIT,
        before: renderState.nextBefore,
      });
      if (generation !== historyGenerationRef.current) return;
      setRenderState((current) => chatEventStateProjector.prepend(current, page));
    } finally {
      if (generation === historyGenerationRef.current) setLoadingOlder(false);
    }
  }, [chatId, loadingOlder, renderState.hasOlder, renderState.nextBefore, historyGenerationRef, historyNeedsRefreshRef, setRenderState]);

  const reattach = useCallback((state: ChatRenderState) => {
    const buffered = bufferedEventsRef.current ?? [];
    resetWindow();
    setRenderState(chatEventStateProjector.append(state, buffered));
  }, [resetWindow, setRenderState]);

  const showAround = useCallback(async (seq: number) => {
    if (historyNeedsRefreshRef.current) return;
    const loaded = renderStateRef.current.events;
    const earliest = loaded.reduce(
      (min, event) => (event.seq ? Math.min(min, event.seq) : min),
      Number.POSITIVE_INFINITY,
    );
    const latest = chatEventStateProjector.latestSequence(loaded);
    if (seq >= earliest && (bufferedEventsRef.current === null || seq <= latest)) return;
    const generation = historyGenerationRef.current;
    setLoadingOlder(true);
    try {
      const page = await chatApi.fetchTranscript(chatId, {
        limit: CHAT_JUMP_CONTEXT_TURNS * 2 + 1,
        anchor: seq,
        newer: CHAT_JUMP_CONTEXT_TURNS,
      });
      if (generation !== historyGenerationRef.current) return;
      const current = renderStateRef.current;
      if (bufferedEventsRef.current === null && (!page.newerFrom || page.newerFrom >= earliest)) {
        // The window meets the loaded conversation: one continuous thread.
        setRenderState(chatEventStateProjector.prepend(current, page));
      } else if (!page.newerFrom) {
        reattach(chatEventStateProjector.fromEvents(page.events, page));
      } else {
        // A gap separates the window from the latest messages. Show the
        // window alone; newer pages close the gap on request.
        bufferedEventsRef.current ??= [];
        setHasNewer(true);
        setRenderState(chatEventStateProjector.fromEvents(page.events, page));
      }
    } finally {
      if (generation === historyGenerationRef.current) setLoadingOlder(false);
    }
  }, [chatId, historyGenerationRef, historyNeedsRefreshRef, reattach, setRenderState]);

  const loadNewer = useCallback(async () => {
    if (bufferedEventsRef.current === null || loadingNewer) return;
    const generation = historyGenerationRef.current;
    const latest = chatEventStateProjector.latestSequence(renderStateRef.current.events);
    setLoadingNewer(true);
    try {
      const page = await chatApi.fetchTranscript(chatId, {
        limit: CHAT_TRANSCRIPT_TURN_PAGE_LIMIT,
        anchor: latest,
        newer: CHAT_TRANSCRIPT_TURN_PAGE_LIMIT,
      });
      if (generation !== historyGenerationRef.current || bufferedEventsRef.current === null) return;
      const extended = chatEventStateProjector.append(renderStateRef.current, page.events);
      if (page.newerFrom) setRenderState(extended);
      else reattach(extended);
    } finally {
      if (generation === historyGenerationRef.current) setLoadingNewer(false);
    }
  }, [chatId, historyGenerationRef, loadingNewer, reattach, setRenderState]);

  return {
    loadingOlder,
    loadOlder,
    hasNewer,
    loadingNewer,
    showAround,
    loadNewer,
    isDetached,
    bufferLiveEvents,
    resetWindow,
    resetOlderLoad,
  };
}
