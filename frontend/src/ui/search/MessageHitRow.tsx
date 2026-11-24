import type { ConversationSearchHit } from "../../api/chat/chatTranscriptApi";
import { chatFindSenderService, type ChatFindSenderRole } from "../../services/chat/chatFindSenderService.ts";
import { searchExcerptService } from "../../services/chat/searchExcerptService.ts";
import { HighlightedText } from "../primitives/HighlightedText";
import { AlertCircle, Bot, MessageSquare, Terminal } from "../primitives/icons";

const avatarClass: Record<ChatFindSenderRole, string> = {
  user: "bg-accent-blue/15 text-accent-blue",
  agent: "bg-tint-strong text-ink-200",
  tool: "bg-tint-strong text-ink-300",
  error: "bg-accent-red/15 text-accent-red",
};

const avatarIcon = {
  user: MessageSquare,
  agent: Bot,
  tool: Terminal,
  error: AlertCircle,
} satisfies Record<ChatFindSenderRole, unknown>;

/** Characters of context kept before an excerpt's first match, so a row
 *  clamped to three lines still shows it. */
const EXCERPT_LEAD = 48;

function formatTime(t: number): string {
  if (!t) return "";
  return new Date(t).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

/**
 * One message search result: who sent it, when, and a clamped excerpt with
 * the matched words highlighted. `context` names where it was found, such as
 * the chat, when results span more than one.
 */
export function MessageHitRow({
  hit,
  selected = false,
  context,
  onSelect,
  ...rest
}: {
  hit: ConversationSearchHit;
  selected?: boolean;
  context?: string;
  onSelect: () => void;
  [attribute: `data-${string}`]: string | number;
}) {
  const sender = chatFindSenderService.describe(hit.kind);
  const Icon = avatarIcon[sender.role];
  const excerpt = searchExcerptService.focus(hit.text, hit.highlights, EXCERPT_LEAD);
  return (
    <button
      type="button"
      {...rest}
      onClick={onSelect}
      aria-current={selected ? "true" : undefined}
      class={`flex w-full gap-2.5 rounded-control border px-2.5 py-2 text-left transition-colors ${
        selected
          ? "border-accent-blue/40 bg-accent-blue/[0.08]"
          : "border-transparent hover:bg-tint-strong"
      }`}
    >
      <span class={`grid h-7 w-7 flex-none place-items-center rounded-full ${avatarClass[sender.role]}`}>
        <Icon class="h-3.5 w-3.5" />
      </span>
      <span class="min-w-0 flex-1">
        <span class="flex items-baseline justify-between gap-2">
          <span class="truncate text-[12.5px] font-semibold text-ink-100">{sender.label}</span>
          <span class="flex-none text-[11px] tabular-nums text-ink-400">{formatTime(hit.t)}</span>
        </span>
        {context && <span class="block truncate text-[11px] text-ink-400">{context}</span>}
        <HighlightedText
          text={excerpt.text}
          spans={excerpt.spans}
          class="mt-0.5 line-clamp-3 break-words text-[12.5px] leading-snug text-ink-200"
        />
      </span>
    </button>
  );
}
