import { API_ROUTES } from "../../config/routes.ts";
import type {
  ChatEvent,
  ChatEventPage,
  TranscriptContentPage,
} from "../../models/chat.ts";
import type { MatchSpan } from "../../models/search.ts";
import { requestJson } from "../apiRequest.ts";
import { CHAT_SEARCH_PAGE_LIMIT } from "../../config/api.ts";

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
  newerFrom?: number;
}

/**
 * One transcript page, newest turns last. `before` pages backwards; `anchor`
 * with `newer` instead ends the page `newer` turns after the turn holding
 * event `anchor`, for a window around a message.
 */
export async function fetchTranscript(
  id: string,
  params: { limit?: number; before?: number; anchor?: number; newer?: number } = {}
): Promise<ChatEventPage> {
  const search = new URLSearchParams();
  if (params.limit) search.set("limit", String(params.limit));
  if (params.before) search.set("before", String(params.before));
  if (params.newer) {
    search.set("anchor", String(params.anchor ?? 0));
    search.set("newer", String(params.newer));
  }
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
    ...(page.newerFrom ? { newerFrom: page.newerFrom } : {}),
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
  /** Type of the matching event: "user", "assistant_text", "thinking",
   *  "tool_use_end" and so on. */
  kind: string;
  /** When the matching event was recorded, in Unix milliseconds. */
  t: number;
  /** Plain-text excerpt, with Markdown syntax already removed. */
  text: string;
  /** Matched words in `text`, as UTF-16 offsets. */
  highlights: MatchSpan[];
}

/** A message match from a search across chats. */
export interface ChatSearchHit extends ConversationSearchHit {
  chatId: string;
}

/** Messages across every chat the caller can read, newest first. Pass the
 *  last hit as `after` for the next page; a short page is the last. */
export function searchChats(query: string, after?: ChatSearchHit): Promise<ChatSearchHit[]> {
  const search = new URLSearchParams({ q: query, limit: String(CHAT_SEARCH_PAGE_LIMIT) });
  if (after) search.set("before", `${after.t}:${after.chatId}:${after.startSeq}`);
  return requestJson<ChatSearchHit[]>("GET", API_ROUTES.chats.searchAll(search.toString()));
}

/** Matches newest first. Pass the last hit's `startSeq` as `before` for the
 *  next page; a short page is the last. */
export function searchTranscript(id: string, query: string, before = 0): Promise<ConversationSearchHit[]> {
  const search = new URLSearchParams({ q: query, limit: String(CHAT_SEARCH_PAGE_LIMIT) });
  if (before > 0) search.set("before", String(before));
  return requestJson<ConversationSearchHit[]>("GET", API_ROUTES.chats.search(id, search.toString()));
}
