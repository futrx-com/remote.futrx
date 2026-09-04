import { useEffect, useRef } from "preact/hooks";
import { CHAT_FIND_SKIP_ATTRIBUTE } from "../../../config/chat.ts";
import type { ChatFind } from "../../../state/hooks/chat/useChatFind";
import { ArrowDown, ArrowUp, Search, X } from "../../primitives/icons";

const stepButtonClass =
  "grid h-6 w-6 flex-none place-items-center rounded text-ink-300 transition-colors " +
  "hover:bg-tint-strong hover:text-ink-50 disabled:opacity-40 disabled:hover:bg-transparent";

/**
 * Find-in-chat, floating over the top of the thread the way an editor's find
 * widget does, so opening it never reflows the messages you are reading.
 *
 * `CHAT_FIND_SKIP_ATTRIBUTE` keeps the bar out of its own results.
 */
export function ChatFindBar({
  find,
  hasUnloadedMessages,
}: {
  find: ChatFind;
  /** Older messages exist on the server, so the thread is not all here yet. */
  hasUnloadedMessages: boolean;
}) {
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!find.open) return;
    // Re-pressing Cmd+F while open should re-target the existing query, the
    // way a browser's find does, rather than do nothing.
    inputRef.current?.focus();
    inputRef.current?.select();
  }, [find.open]);

  if (!find.open) return null;

  const { status } = find;
  const canStep = status.kind === "matched";

  return (
    <div
      {...{ [CHAT_FIND_SKIP_ATTRIBUTE]: true }}
      class="absolute right-3 top-3 z-30 flex max-w-[calc(100%-1.5rem)] flex-col gap-1
             rounded-card border border-line bg-raised px-2 py-1.5 shadow-pop"
      role="search"
      aria-label="Find in chat"
    >
      <div class="flex items-center gap-1.5">
        <Search class="h-3.5 w-3.5 flex-none text-ink-400" />
        <input
          ref={inputRef}
          value={find.query}
          onInput={(event) => find.setQuery((event.currentTarget as HTMLInputElement).value)}
          onKeyDown={(event) => {
            // Escape is not handled here: `useChatFind` claims it for the
            // whole thread, so closing works with the cursor anywhere.
            if (event.key === "Enter") {
              event.preventDefault();
              event.shiftKey ? find.previous() : find.next();
            }
          }}
          placeholder="Find in chat"
          class="min-w-0 flex-1 bg-transparent text-[12.5px] text-ink-100 placeholder:text-ink-400 focus:outline-none"
          autocomplete="off"
          spellcheck={false}
          aria-label="Find in chat"
        />
        <span
          class={`flex-none tabular-nums text-[11px] ${
            status.kind === "empty" ? "text-accent-red" : "text-ink-400"
          }`}
          aria-live="polite"
        >
          {status.kind === "idle"
            ? ""
            : status.kind === "empty"
              ? "No results"
              : `${status.position} of ${status.total}`}
        </span>
        <button
          type="button"
          onClick={find.previous}
          disabled={!canStep}
          class={stepButtonClass}
          aria-label="Previous match"
          title="Previous match (Shift+Enter)"
        >
          <ArrowUp class="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          onClick={find.next}
          disabled={!canStep}
          class={stepButtonClass}
          aria-label="Next match"
          title="Next match (Enter)"
        >
          <ArrowDown class="h-3.5 w-3.5" />
        </button>
        <button
          type="button"
          onClick={find.close}
          class={stepButtonClass}
          aria-label="Close find"
          title="Close (Esc)"
        >
          <X class="h-3.5 w-3.5" />
        </button>
      </div>

      {hasUnloadedMessages && (
        <p class="text-[10.5px] leading-tight text-ink-400">
          Searching the messages loaded so far — load older ones to include them.
        </p>
      )}
    </div>
  );
}
