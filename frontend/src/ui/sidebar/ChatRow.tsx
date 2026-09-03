import type { ChatMeta } from "../../models/chat";
import { relativeTimeService } from "../../services/platform/relativeTimeService.ts";
import { Eye, EyeOff, GitFork, Loader, MessageSquare, X } from "../primitives/icons";

const rowActionClass =
  "w-7 grid place-items-center rounded-control text-ink-400 transition-colors " +
  "hover:bg-tint-strong hover:text-ink-50";

export function ChatRow({
  chat,
  active,
  onSelect,
  onDelete,
  onToggleUnread,
  onFork,
}: {
  chat: ChatMeta;
  active: boolean;
  onSelect: () => void;
  onDelete: (event: Event) => void;
  onToggleUnread: (event: Event) => void;
  onFork: (event: Event) => void;
}) {
  const rawUnread = (chat.lastMessageAt || 0) > (chat.lastReadAt || 0);
  const unread = !active && !chat.running && rawUnread;

  return (
    <div
      class={`group flex items-center rounded-control transition-colors
              ${active ? "bg-tint-active" : "hover:bg-tint"}`}
    >
      <button
        type="button"
        onClick={onSelect}
        class="flex min-w-0 flex-1 items-center gap-2 py-1.5 pl-2 pr-2 text-left"
      >
        <span class="grid h-4 w-4 flex-none place-items-center">
          {chat.running ? (
            <Loader class="h-3.5 w-3.5 animate-spin text-accent-blue" />
          ) : unread ? (
            <span class="h-2 w-2 rounded-full bg-accent-green" title="Unread" />
          ) : (
            <MessageSquare class={`h-3.5 w-3.5 ${active ? "text-ink-100" : "text-ink-400"}`} />
          )}
        </span>
        <span
          class={`min-w-0 flex-1 truncate text-[13px] leading-5
                  ${active ? "font-medium text-ink-50" : unread ? "text-ink-100" : "text-ink-200"}`}
        >
          {chat.title || "Untitled"}
        </span>
      </button>

      {/* Age hands its slot to the row actions on hover — same pattern the row
          uses on touch, where the actions are simply always present. */}
      <span
        class="pointer-events-none hidden flex-none pr-2.5 text-[11px] tabular-nums text-ink-400
               md:block md:group-hover:hidden md:group-focus-within:hidden"
        title={relativeTimeService.ago(chat.lastMessageAt)}
      >
        {relativeTimeService.shortAgo(chat.lastMessageAt)}
      </span>

      <div
        class="flex flex-none items-stretch gap-0.5 pr-1
               md:hidden md:group-hover:flex md:group-focus-within:flex"
      >
        <button
          type="button"
          onClick={onToggleUnread}
          class={rowActionClass}
          aria-label={rawUnread ? `Mark ${chat.title || "chat"} read` : `Mark ${chat.title || "chat"} unread`}
          title={rawUnread ? "Mark read" : "Mark unread"}
        >
          {rawUnread ? <Eye class="h-3.5 w-3.5" /> : <EyeOff class="h-3.5 w-3.5" />}
        </button>
        <button
          type="button"
          onClick={onFork}
          class={rowActionClass}
          aria-label={`Fork ${chat.title || "chat"}`}
          title="Fork from last message"
        >
          <GitFork class="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          onClick={onDelete}
          class={`${rowActionClass} hover:bg-accent-red/10 hover:text-accent-red`}
          aria-label={`Delete ${chat.title || "chat"}`}
          title="Delete chat"
        >
          <X class="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
}
