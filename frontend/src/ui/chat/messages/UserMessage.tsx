import { RotateCcw } from "../../primitives/icons";
import { getTextAlignClass, getTextDirection } from "../markdown/bidi";

export function UserMessage({
  text,
  t,
  onRewind,
}: {
  text: string;
  t: number;
  onRewind?: (t: number, text: string) => void;
}) {
  const dir = getTextDirection(text);
  const align = getTextAlignClass(text);

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
          {text}
        </div>
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
