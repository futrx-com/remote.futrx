import { useEffect, useRef } from "preact/hooks";
import { CHAT_FIND_SKIP_ATTRIBUTE } from "../../../config/chat.ts";
import type { ChatFind } from "../../../state/hooks/chat/useChatFind";
import { shortcutService } from "../../../services/platform/shortcutService.ts";
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
}: {
  find: ChatFind;
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
            } else if (shortcutService.isFind(event)) {
              // Cmd/Ctrl+F again re-selects the query, as a browser's find
              // does, and stays with this bar rather than opening another.
              event.preventDefault();
              event.stopPropagation();
              event.currentTarget.select();
            }
          }}
          placeholder="Find in chat"
          class="min-w-0 flex-1 bg-transparent text-[12.5px] text-ink-100 placeholder:text-ink-400 focus:outline-none"
          maxLength={512}
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
              : status.kind === "loading" ? "Searching…"
              : status.kind === "error" ? "Search failed"
              : `${status.position} of ${status.total}${status.total === 100 ? "+" : ""}`}
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

      <p class="text-[10.5px] leading-tight text-ink-400">
        Searching the full conversation. {status.kind === "matched" && "Results are matching messages and tool outputs."}
      </p>
      {status.kind === "error" && <p role="alert" class="text-xs text-accent-red">{status.message}</p>}
      {find.selectedHit && (
        <section aria-label="Search result preview" class="mt-2 max-h-[55vh] w-[min(36rem,80vw)] overflow-auto border-t border-line pt-2 text-xs">
          <p class="mb-2 whitespace-pre-wrap break-words text-ink-100">{find.selectedHit.text}</p>
          {find.previewLoading && <p class="text-ink-400">Loading conversation turn…</p>}
          {find.previewError && <p role="alert" class="text-accent-red">{find.previewError}</p>}
          {find.preview.map((event, index) => {
            const text = event.type === "tool_use_end" ? event.output : (event.type === "user" || event.type === "assistant_text") ? event.text : "";
            if (!text) return null;
            const label = event.type === "user" ? "You" : event.type === "tool_use_end" ? "Tool output" : "Assistant";
            return <div key={`${event.seq}-${index}`} class="mb-3 rounded-control bg-surface p-2">
              <p class="mb-1 font-medium text-ink-400">{label}</p>
              <p class="whitespace-pre-wrap break-words text-ink-200">{text.slice(0, 12000)}</p>
              {(text.length > 12000 || (event.type === "tool_use_end" && event.outputTruncated)) && <p class="mt-1 text-ink-400">Preview excerpt</p>}
            </div>;
          })}
        </section>
      )}
    </div>
  );
}
