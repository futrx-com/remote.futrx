import { useEffect, useRef, useState } from "preact/hooks";
import { CHAT_TITLE_MAX_LENGTH } from "../../config/chat";
import type { ChatMeta } from "../../models/chat";
import { relativeTimeService } from "../../services/platform/relativeTimeService.ts";
import { shortcutService } from "../../services/platform/shortcutService.ts";
import { Edit, Eye, EyeOff, GitFork, Loader, MessageSquare, X } from "../primitives/icons";

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
  onRename,
}: {
  chat: ChatMeta;
  active: boolean;
  onSelect: () => void;
  onDelete: (event: Event) => void;
  onToggleUnread: (event: Event) => void;
  onFork: (event: Event) => void;
  onRename: (title: string) => void;
}) {
  const rawUnread = (chat.lastMessageAt || 0) > (chat.lastReadAt || 0);
  const unread = !active && !chat.running && rawUnread;
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  // Enter and Escape end the edit by unmounting the input, and a blur can
  // follow; this keeps that blur from saving twice or undoing a cancel.
  const finished = useRef(false);

  useEffect(() => {
    if (!editing) return;
    inputRef.current?.focus();
    inputRef.current?.select();
  }, [editing]);

  function startEditing(event: Event) {
    event.stopPropagation();
    finished.current = false;
    setDraft(chat.title || "");
    setEditing(true);
  }

  function finishEditing(save: boolean) {
    if (finished.current) return;
    finished.current = true;
    // Read the field itself: a paste followed at once by Enter can land before
    // the draft state has re-rendered, and the save would drop it.
    const value = inputRef.current?.value ?? draft;
    setEditing(false);
    if (save) onRename(value);
  }

  function onEditKeyDown(event: KeyboardEvent) {
    if (event.key === "Enter" && !event.isComposing) {
      event.preventDefault();
      finishEditing(true);
    } else if (shortcutService.isDismiss(event)) {
      // Answer Escape here so it cancels the rename rather than also closing
      // whatever surface holds the sidebar.
      event.preventDefault();
      event.stopPropagation();
      finishEditing(false);
    }
  }

  return (
    <div
      class={`group flex items-center rounded-control transition-colors
              ${active ? "bg-tint-active" : "hover:bg-tint"}`}
    >
      {editing ? (
        <div class="flex min-w-0 flex-1 items-center gap-2 py-1 pl-2 pr-1">
          <span class="grid h-4 w-4 flex-none place-items-center">
            <Edit class="h-3.5 w-3.5 text-ink-400" />
          </span>
          <input
            ref={inputRef}
            value={draft}
            maxLength={CHAT_TITLE_MAX_LENGTH}
            aria-label="Chat name"
            onInput={(event) => setDraft((event.currentTarget as HTMLInputElement).value)}
            onKeyDown={onEditKeyDown}
            onBlur={() => finishEditing(true)}
            class="h-6 min-w-0 flex-1 rounded-control border border-accent-blue/60 bg-inset px-1.5
                   text-[13px] leading-5 text-ink-50 focus:outline-none"
          />
        </div>
      ) : (
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
      )}

      {/* Age hands its slot to the row actions on hover — same pattern the row
          uses on touch, where the actions are simply always present. The
          rename field takes the whole row while it is open. */}
      {!editing && (
        <>
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
              onClick={startEditing}
              class={rowActionClass}
              aria-label={`Rename ${chat.title || "chat"}`}
              title="Rename chat"
            >
              <Edit class="h-3.5 w-3.5" />
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
        </>
      )}
    </div>
  );
}
