import { API_ROUTES } from "../../config/routes.ts";
import type { ChatEvent, ChatEventPage } from "../../models/chat.ts";
import { requestJson } from "../apiRequest.ts";

interface ChatTranscriptTurnPayload {
  id: string;
  startSeq: number;
  endSeq: number;
  events: ChatEvent[];
}

interface ChatTranscriptPagePayload {
  turns: ChatTranscriptTurnPayload[];
  nextBefore?: number;
  lastSeq: number;
  hasMore: boolean;
}

export async function fetchTranscript(
  id: string,
  params: { limit?: number; before?: number } = {}
): Promise<ChatEventPage> {
  const search = new URLSearchParams();
  if (params.limit) search.set("limit", String(params.limit));
  if (params.before) search.set("before", String(params.before));
  const query = search.toString();
  const page = await requestJson<ChatTranscriptPagePayload>(
    "GET",
    API_ROUTES.chats.transcript(id, query)
  );
  return transcriptPageToEventPage(page);
}

function transcriptPageToEventPage(page: ChatTranscriptPagePayload): ChatEventPage {
  return {
    events: page.turns.flatMap((turn) => turn.events),
    nextBefore: page.nextBefore,
    lastSeq: page.lastSeq,
    hasMore: page.hasMore,
  };
}
