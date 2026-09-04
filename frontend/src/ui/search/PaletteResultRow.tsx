import type { SearchHit } from "../../models/search";
import { relativeTimeService } from "../../services/platform/relativeTimeService.ts";
import { HighlightedText } from "../primitives/HighlightedText";
import { CornerDownLeft, Folder, Loader, MessageSquare } from "../primitives/icons";

/**
 * One ranked result in the palette. The sidebar's `SearchResultRow` shows the
 * same hit for a surface you browse; this one is sized for a surface you drive
 * from the keyboard, so it carries the Enter affordance and says why it matched
 * rather than the chat's model and age.
 *
 * `active` is the keyboard cursor, not the open chat -- the palette highlights
 * where Enter would land.
 */
export function PaletteResultRow({
  hit,
  active,
  reason,
  onActivate,
  onSelect,
}: {
  hit: SearchHit;
  active: boolean;
  /** Why this chat matched, or null when its title already shows it. */
  reason: string | null;
  /** The pointer moved onto the row: take the keyboard cursor with it. */
  onActivate: () => void;
  onSelect: () => void;
}) {
  const chat = hit.doc.chat;

  return (
    <button
      type="button"
      // The palette's scroll-into-view looks the cursor up by this attribute.
      data-active={active ? "true" : "false"}
      onMouseMove={onActivate}
      onClick={onSelect}
      class={`flex w-full items-start gap-2.5 rounded-card px-3 py-2 text-left transition-colors
              ${active ? "bg-accent-blue/[0.14]" : "hover:bg-tint"}`}
    >
      {chat.running ? (
        <Loader class="mt-0.5 h-4 w-4 flex-none animate-spin text-accent-blue" />
      ) : (
        <MessageSquare
          class={`mt-0.5 h-4 w-4 flex-none ${active ? "text-accent-blue" : "text-ink-400"}`}
        />
      )}

      <span class="min-w-0 flex-1">
        <HighlightedText
          text={chat.title || "Untitled"}
          spans={hit.titleSpans}
          class={`block truncate text-[13.5px] leading-snug ${
            active ? "text-ink-50" : "text-ink-100"
          }`}
        />
        <span class="mt-0.5 flex items-center gap-1.5 text-[11px] text-ink-400">
          {hit.doc.project ? (
            <>
              <Folder class="h-3 w-3 flex-none" />
              <span class="truncate">{hit.doc.project.name}</span>
            </>
          ) : (
            <span class="truncate">Unassigned</span>
          )}
          <span aria-hidden="true">·</span>
          <span class="flex-none">{relativeTimeService.ago(chat.lastMessageAt)}</span>
          {reason && (
            <>
              <span aria-hidden="true">·</span>
              <span class="truncate text-ink-500">{reason}</span>
            </>
          )}
        </span>
      </span>

      {active && <CornerDownLeft class="mt-1 h-3.5 w-3.5 flex-none text-accent-blue" />}
    </button>
  );
}
