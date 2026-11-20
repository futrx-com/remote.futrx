import { API_ROUTES } from "../../config/routes.ts";
import type {
  ChatEvent,
  ChatEventPage,
  TranscriptContentPage,
} from "../../models/chat.ts";
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

export async function fetchTranscriptContent(
  chatId: string,
  contentId: string,
  after = 0,
): Promise<TranscriptContentPage> {
  const search = new URLSearchParams({ id: contentId });
  if (after > 0) search.set("after", String(after));
  return requestJson<TranscriptContentPage>(
    "GET",
    API_ROUTES.chats.transcriptContent(chatId, search.toString()),
  );
}

export async function fetchFullTranscriptContent(
  chatId: string,
  contentId: string,
): Promise<string> {
  let content = "";
  let after = 0;
  for (;;) {
    const page = await fetchTranscriptContent(chatId, contentId, after);
    content += page.content;
    if (page.complete) return content;
    if (!page.nextAfter || page.nextAfter <= after) {
      throw new Error("invalid transcript content cursor");
    }
    after = page.nextAfter;
  }
}

export interface ConversationSearchHit {
  turnId: string;
  itemId: string;
  startSeq: number;
  endSeq: number;
  text: string;
}

export function searchTranscript(id: string, query: string): Promise<ConversationSearchHit[]> {
  const search = new URLSearchParams({ q: query, limit: "100" });
  return requestJson<ConversationSearchHit[]>("GET", API_ROUTES.chats.search(id, search.toString()));
}
