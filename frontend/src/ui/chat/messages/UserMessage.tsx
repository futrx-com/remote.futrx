import { useMemo } from "preact/hooks";
import { RotateCcw } from "../../primitives/icons";
import { chatAttachmentService } from "../../../services/chat/chatAttachmentService.ts";
import { getTextAlignClass, getTextDirection } from "../markdown/bidi";
import { AttachmentPreviews } from "./AttachmentPreviews";

export function UserMessage({
  text,
  t,
  chatId,
  cwd,
  onRewind,
}: {
  text: string;
  t: number;
  chatId?: string;
  cwd?: string;
  onRewind?: (t: number, text: string) => void;
}) {
  // Strip the composer-injected "Attached files:" list and render the
  // uploads as preview chips instead of leaking the raw paths into the
  // message bubble. Older messages (no chatId) keep their raw text.
  const { message, paths } = useMemo(
    () => chatAttachmentService.parseAttachedPaths(text),
    [text],
  );
  const hasAttachments = paths.length > 0;
  const displayText = hasAttachments ? message : text;
  const dir = getTextDirection(displayText);
  const align = getTextAlignClass(displayText);

  return (
    <div class="group flex min-w-0 justify-end">
      <div class="max-w-[92%] sm:max-w-[78%] min-w-0 flex flex-col items-end gap-1.5">
        <div
          dir={dir}
          class={`codex-user-bubble max-w-full rounded-panel rounded-br-control border border-line
                    bg-tint-strong px-3.5 py-2.5 text-[14px] leading-relaxed text-ink-100
                    whitespace-pre-wrap break-words [overflow-wrap:anywhere] ${align}`}
          style={{ unicodeBidi: "plaintext" }}
        >
          {displayText}
        </div>
        {hasAttachments && (
          <AttachmentPreviews paths={paths} chatId={chatId} cwd={cwd} />
        )}
        {onRewind && (
          <button
            type="button"
            onClick={() => onRewind(t, text)}
            class="inline-flex h-7 items-center gap-1.5 rounded-control px-2 text-[12px]
                   text-ink-400 transition-colors hover:bg-tint-strong hover:text-ink-100
                   md:opacity-0 md:group-hover:opacity-100 md:group-focus-within:opacity-100"
            title="Rewind and edit from here"
          >
            <RotateCcw class="w-3.5 h-3.5" />
            Rewind
          </button>
        )}
      </div>
    </div>
  );
}
