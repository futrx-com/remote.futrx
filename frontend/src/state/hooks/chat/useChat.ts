import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { chatApi } from "../../../api/chatApi";
import { CHAT_EVENT_PAGE_LIMIT } from "../../../config/api.ts";
import type { ChatStream } from "../../../types/chatApi";
import type {
  ChatEvent,
  ChatEventPage,
  ChatMeta,
  ChatRenderState,
  ChatStatus,
  PromptOutcome,
} from "../../../models/chat";
import { chatEventStateProjector } from "./chatEventStateProjector";
import type { ChatMessageBlock } from "../../../models/chatMessage";

interface UseChatResult {
  meta: ChatMeta | null;
  blocks: ChatMessageBlock[];
  eventCount: number;
  hasOlder: boolean;
  loadingOlder: boolean;
  status: ChatStatus;
  error: string | null;
  canSendPrompt: boolean;
  sendPrompt: (text: string, clientId?: string) => boolean;
  promptOutcome: PromptOutcome | null;
  cancel: () => void;
  rewind: (beforeT: number) => Promise<ChatEventPage>;
  loadOlder: () => Promise<void>;
  refreshMeta: () => Promise<void>;
}

/**
 * useChat — load chat metadata, then open a streaming WS. The server replays
 * history over the socket before live events, which avoids gaps between an
 * HTTP history fetch and the WS subscription.
 */
export function useChat(chatId: string): UseChatResult {
  const [meta, setMeta] = useState<ChatMeta | null>(null);
  const [renderState, setRenderState] = useState<ChatRenderState>(() =>
    chatEventStateProjector.empty()
  );
  const [status, setStatus] = useState<ChatStatus>("loading");
  const [error, setError] = useState<string | null>(null);
  const [wsReady, setWsReady] = useState(false);
  // True once this connection has received its sync event. Until then the run
  // state is unknown (the HTTP snapshot may be stale), so sends are held.
  const [synced, setSynced] = useState(false);
  const [promptOutcome, setPromptOutcome] = useState<PromptOutcome | null>(null);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const streamRef = useRef<ChatStream | null>(null);
  const pendingEventsRef = useRef<ChatEvent[]>([]);
  const pendingFrameRef = useRef<number | null>(null);
  const lastSeqRef = useRef(0);

  // The batcher reaches state only through refs and setState updaters, so these
  // three close over nothing that can go stale and take no dependencies. They
  // are wrapped because callers already capture them: rewind's useCallback
  // below pins whichever clearPendingEvents existed when it was created, and
  // chatId never changes for a given instance (ChatContainer is keyed on it and
  // remounts), so that capture lasts the whole session.
  const clearPendingEvents = useCallback(() => {
    if (pendingFrameRef.current !== null) {
      cancelAnimationFrame(pendingFrameRef.current);
      pendingFrameRef.current = null;
    }
    pendingEventsRef.current = [];
  }, []);

  const flushPendingEvents = useCallback(() => {
    pendingFrameRef.current = null;
    const events = pendingEventsRef.current;
    if (events.length === 0) return;
    pendingEventsRef.current = [];
    lastSeqRef.current = Math.max(
      lastSeqRef.current,
      chatEventStateProjector.latestSequence(events)
    );
    setRenderState((current) => chatEventStateProjector.append(current, events));
    setStatus((current) => chatEventStateProjector.statusAfter(events[events.length - 1], current));
  }, []);

  const enqueueEvent = useCallback((event: ChatEvent) => {
    pendingEventsRef.current.push(event);
    if (pendingFrameRef.current === null) {
      pendingFrameRef.current = requestAnimationFrame(flushPendingEvents);
    }
  }, [flushPendingEvents]);

  // Load metadata when chat id changes.
  useEffect(() => {
    let cancelled = false;
    setStatus("loading");
    clearPendingEvents();
    setRenderState(chatEventStateProjector.empty());
    setMeta(null);
    setError(null);
    setWsReady(false);
    setSynced(false);
    setPromptOutcome(null);
    setLoadingOlder(false);
    lastSeqRef.current = 0;

    (async () => {
      try {
        const [m, page] = await Promise.all([
          chatApi.fetch(chatId),
          chatApi.fetchEvents(chatId, { limit: CHAT_EVENT_PAGE_LIMIT }),
        ]);
        if (cancelled) return;
        lastSeqRef.current = Math.max(
          page.lastSeq,
          chatEventStateProjector.latestSequence(page.events)
        );
        setRenderState(chatEventStateProjector.fromEvents(page.events, page));
        setMeta(m);
        // The server reports whether a run holds the lock right now. Seeding
        // from it keeps queued prompts from firing into a mid-run chat before
        // the socket's sync event corrects the status.
        setStatus(m.running ? "streaming" : "ready");
      } catch (e) {
        if (!cancelled) {
          setError((e as Error).message);
          setStatus("error");
        }
      }
    })();

    return () => { cancelled = true; };
  }, [chatId]);

  // Open WS once the latest page is loaded. Reconnects request only events
  // after the latest sequence this client has already applied.
  useEffect(() => {
    if (!meta || meta.id !== chatId) return;
    const streamChatId = meta.id;
    setWsReady(false);

    const stream = chatApi.openStream(
      streamChatId,
      () => lastSeqRef.current,
      {
        onOpen: () => {
          if (streamRef.current !== stream) return;
          setError(null);
          clearPendingEvents();
          setSynced(false);
          setWsReady(true);
        },
        onEvent: (event) => {
          if (streamRef.current !== stream) return;
          if (event.type === "sync") {
            setSynced(true);
            setStatus(event.running ? "streaming" : "ready");
            return;
          }
          if (
            event.type === "system" &&
            (event.subtype === "prompt_accepted" || event.subtype === "prompt_rejected")
          ) {
            // Transient per-connection ack for a tracked prompt. Routed to the
            // queue instead of the projector: it is not transcript content and
            // must not influence the streaming status.
            const clientId = event.data?.clientId;
            if (typeof clientId === "string" && clientId) {
              setPromptOutcome({ clientId, accepted: event.subtype === "prompt_accepted" });
            }
            return;
          }
          enqueueEvent(event);
        },
        onClose: () => {
          if (streamRef.current !== stream) return;
          setWsReady(false);
          setSynced(false);
        },
      }
    );
    streamRef.current = stream;

    return () => {
      if (streamRef.current === stream) streamRef.current = null;
      setWsReady(false);
      setSynced(false);
      clearPendingEvents();
      stream.close();
    };
  }, [meta?.id, chatId]);

  const sendPrompt = useCallback((text: string, clientId?: string) => {
    const stream = streamRef.current;
    if (!wsReady || !synced || !stream?.isOpen) return false;
    if (status !== "ready") return false;
    setStatus("streaming");
    stream.sendPrompt(text, clientId);
    return true;
  }, [status, wsReady, synced]);

  const cancel = useCallback(() => {
    const stream = streamRef.current;
    if (stream?.isOpen) stream.cancel();
  }, []);

  const rewind = useCallback(async (beforeT: number) => {
    const res = await chatApi.rewind(chatId, beforeT);
    clearPendingEvents();
    lastSeqRef.current = Math.max(
      res.lastSeq,
      chatEventStateProjector.latestSequence(res.events)
    );
    setRenderState(chatEventStateProjector.fromEvents(res.events, res));
    setStatus("ready");
    return res;
  }, [chatId]);

  const loadOlder = useCallback(async () => {
    if (loadingOlder || !renderState.hasOlder || !renderState.nextBefore) return;
    setLoadingOlder(true);
    try {
      const page = await chatApi.fetchEvents(chatId, {
        limit: CHAT_EVENT_PAGE_LIMIT,
        before: renderState.nextBefore,
      });
      setRenderState((current) => chatEventStateProjector.prepend(current, page));
    } finally {
      setLoadingOlder(false);
    }
  }, [chatId, loadingOlder, renderState.hasOlder, renderState.nextBefore]);

  const refreshMeta = useCallback(async () => {
    if (!chatId) return;
    try {
      const m = await chatApi.fetch(chatId);
      setMeta(m);
    } catch {}
  }, [chatId]);

  return {
    meta,
    blocks: renderState.blocks,
    eventCount: renderState.eventCount,
    hasOlder: renderState.hasOlder,
    loadingOlder,
    status,
    error,
    canSendPrompt: wsReady && synced && status === "ready",
    sendPrompt,
    promptOutcome,
    cancel,
    rewind,
    loadOlder,
    refreshMeta,
  };
}
